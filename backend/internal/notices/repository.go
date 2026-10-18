package notices

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, files.ErrInvalid) {
		return ErrInvalid
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22P02", "22007", "22008":
			return ErrInvalid
		}
	}
	return err
}

const liveSQL = "(n.status = 'published' AND n.publish_at <= now() AND (n.expires_at IS NULL OR n.expires_at > now()))"
const utcFormat = `'YYYY-MM-DD"T"HH24:MI:SS"Z"'`
const noticeSelect = "n.id,n.title_en,n.title_bn,n.body_en,n.body_bn,n.priority,array_to_json(n.audience_roles)::text AS roles_json,n.status," +
	"to_char(n.publish_at AT TIME ZONE 'UTC'," + utcFormat + ") AS publish_at,to_char(n.expires_at AT TIME ZONE 'UTC'," + utcFormat + ") AS expires_at," +
	liveSQL + " AS live,COALESCE(NULLIF(au.display_name,''),au.email) AS author_name,to_char(n.updated_at AT TIME ZONE 'UTC'," + utcFormat + ") AS updated_at"

func noticeBase(db *gorm.DB) *gorm.DB {
	return db.Table("notices n").Joins("JOIN users au ON au.id = n.created_by")
}

func searchLike(v string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(v)) + "%"
}

// hydrate decodes audience roles and loads targets and attachments in batches.
func hydrate(db *gorm.DB, items []Notice) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	index := map[string]int{}
	for i := range items {
		ids[i] = items[i].ID
		index[items[i].ID] = i
		items[i].Targets, items[i].Attachments = []Target{}, []Attachment{}
		_ = json.Unmarshal([]byte(items[i].RolesJSON), &items[i].AudienceRoles)
	}
	var targets []struct {
		NoticeID string
		Target
	}
	if err := db.Table("notice_targets t").Joins("JOIN academic_classes c ON c.id = t.class_id").Joins("LEFT JOIN academic_sections s ON s.id = t.section_id").
		Select("t.notice_id,t.class_id,t.section_id,c.name AS class_name,COALESCE(s.name,'') AS section_name").Where("t.notice_id IN ?", ids).Order("c.display_order,c.name,s.name").Find(&targets).Error; err != nil {
		return err
	}
	for _, t := range targets {
		items[index[t.NoticeID]].Targets = append(items[index[t.NoticeID]].Targets, t.Target)
	}
	var attachments []struct {
		NoticeID string
		Attachment
	}
	if err := db.Table("notice_attachments a").Joins("JOIN file_objects f ON f.id = a.file_id").Select("a.notice_id,f.id,f.original_name AS name,f.content_type,f.size_bytes AS size").Where("a.notice_id IN ?", ids).Order("a.position").Find(&attachments).Error; err != nil {
		return err
	}
	for _, a := range attachments {
		items[index[a.NoticeID]].Attachments = append(items[index[a.NoticeID]].Attachments, a.Attachment)
	}
	return nil
}

func page[T any](base *gorm.DB, q Query, sel string, order string, after func([]T) error) (Page[T], error) {
	out := Page[T]{Items: []T{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := base.Select(sel).Order(order).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&out.Items).Error; err != nil {
		return out, err
	}
	return out, after(out.Items)
}

func filterNotices(base *gorm.DB, q Query) *gorm.DB {
	if q.Search != "" {
		p := searchLike(q.Search)
		base = base.Where("(lower(n.title_en) LIKE ? OR n.title_bn LIKE ? OR lower(n.body_en) LIKE ? OR n.body_bn LIKE ?)", p, "%"+q.Search+"%", p, "%"+q.Search+"%")
	}
	if q.Priority != "" {
		base = base.Where("n.priority = ?", q.Priority)
	}
	return base
}

func (r *GormRepository) ListManaged(ctx context.Context, q Query) (Page[Notice], error) {
	db := r.db.WithContext(ctx)
	base := filterNotices(noticeBase(db), q)
	switch q.Status {
	case "draft", "archived":
		base = base.Where("n.status = ?", q.Status)
	case "published":
		base = base.Where(liveSQL)
	case "scheduled":
		base = base.Where("n.status = 'published' AND n.publish_at > now()")
	case "expired":
		base = base.Where("n.status = 'published' AND n.expires_at <= now()")
	}
	return page(base, q, noticeSelect, "n.updated_at DESC, n.id", func(items []Notice) error { return hydrate(db, items) })
}

func (r *GormRepository) Feed(ctx context.Context, actor string, q Query) (Page[Notice], error) {
	db := r.db.WithContext(ctx)
	base := filterNotices(noticeBase(db).Joins("JOIN users u ON u.id = ? AND u.is_active", actor).Where(liveSQL).Where(AudiencePredicate("n", "notice_targets", "notice_id")), q)
	return page(base, q, noticeSelect, "CASE n.priority WHEN 'urgent' THEN 0 WHEN 'important' THEN 1 ELSE 2 END, n.publish_at DESC, n.id", func(items []Notice) error { return hydrate(db, items) })
}

func (r *GormRepository) get(db *gorm.DB, actor, id string, manager bool) (Notice, error) {
	var n Notice
	base := noticeBase(db).Where("n.id = ?", id)
	if !manager {
		base = base.Joins("JOIN users u ON u.id = ? AND u.is_active", actor).Where(liveSQL).Where(AudiencePredicate("n", "notice_targets", "notice_id"))
	}
	if err := base.Select(noticeSelect).Take(&n).Error; err != nil {
		return Notice{}, dbErr(err)
	}
	items := []Notice{n}
	err := hydrate(db, items)
	return items[0], err
}

func (r *GormRepository) Get(ctx context.Context, actor, id string, manager bool) (Notice, error) {
	return r.get(r.db.WithContext(ctx), actor, id, manager)
}

func replaceTargets(tx *gorm.DB, table, fk, id string, targets []Target) error {
	if err := tx.Exec("DELETE FROM "+table+" WHERE "+fk+" = ?", id).Error; err != nil {
		return err
	}
	for _, t := range targets {
		if t.SectionID != nil {
			var n int64
			if err := tx.Table("academic_sections").Where("id = ? AND class_id = ?", *t.SectionID, t.ClassID).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return ErrInvalid
			}
		}
		if err := tx.Exec("INSERT INTO "+table+"("+fk+",class_id,section_id) VALUES (?,?,?)", id, t.ClassID, t.SectionID).Error; err != nil {
			return dbErr(err)
		}
	}
	return nil
}

func lockedStatus(tx *gorm.DB, table, id string) (string, error) {
	var status string
	err := tx.Table(table).Select("status").Where("id = ?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&status).Error
	return status, dbErr(err)
}

// Save creates a draft or edits a draft or published notice. Attachments are
// replaced by the requested list: removed files are released for cleanup and
// new ones must be the actor's own unattached notice uploads.
func (r *GormRepository) Save(ctx context.Context, actor, id string, in NoticeInput) (Notice, error) {
	var out Notice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roles := pgTextArray(in.AudienceRoles)
		if id == "" {
			if err := tx.Raw("INSERT INTO notices(title_en,title_bn,body_en,body_bn,priority,audience_roles,expires_at,created_by,updated_by) VALUES (?,?,?,?,?,?::text[],?,?,?) RETURNING id",
				in.TitleEN, in.TitleBN, in.BodyEN, in.BodyBN, in.Priority, roles, in.ExpiresAt, actor, actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			status, err := lockedStatus(tx, "notices", id)
			if err != nil {
				return err
			}
			if status == "archived" {
				return ErrConflict
			}
			if err := tx.Exec("UPDATE notices SET title_en=?,title_bn=?,body_en=?,body_bn=?,priority=?,audience_roles=?::text[],expires_at=?,updated_by=?,updated_at=now() WHERE id=?",
				in.TitleEN, in.TitleBN, in.BodyEN, in.BodyBN, in.Priority, roles, in.ExpiresAt, actor, id).Error; err != nil {
				return dbErr(err)
			}
		}
		if err := replaceTargets(tx, "notice_targets", "notice_id", id, in.Targets); err != nil {
			return err
		}
		var current []string
		if err := tx.Table("notice_attachments").Where("notice_id = ?", id).Pluck("file_id", &current).Error; err != nil {
			return err
		}
		removed, added := []string{}, []string{}
		for _, f := range current {
			if !slices.ContainsFunc(in.FileIDs, func(v string) bool { return strings.EqualFold(v, f) }) {
				removed = append(removed, f)
			}
		}
		for _, f := range in.FileIDs {
			if !slices.ContainsFunc(current, func(v string) bool { return strings.EqualFold(v, f) }) {
				added = append(added, f)
			}
		}
		if err := tx.Exec("DELETE FROM notice_attachments WHERE notice_id = ?", id).Error; err != nil {
			return err
		}
		if err := files.UnlinkTx(tx, removed); err != nil {
			return err
		}
		if err := files.LinkTx(tx, actor, "notice", added); err != nil {
			return ErrInvalid
		}
		for i, f := range in.FileIDs {
			if err := tx.Exec("INSERT INTO notice_attachments(notice_id,file_id,position) VALUES (?,?,?)", id, f, i).Error; err != nil {
				return dbErr(err)
			}
		}
		var err error
		if out, err = r.get(tx, actor, id, true); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "notice.saved", "notice", id, map[string]any{"priority": in.Priority, "roles": in.AudienceRoles, "targets": len(in.Targets), "attachments": len(in.FileIDs)})
	})
	return out, dbErr(err)
}

func (r *GormRepository) Publish(ctx context.Context, actor, id string, at *time.Time) (Notice, error) {
	var out Notice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockedStatus(tx, "notices", id)
		if err != nil {
			return err
		}
		if status == "archived" {
			return ErrConflict
		}
		var when any
		if at != nil {
			when = at.UTC()
		}
		if err := tx.Exec("UPDATE notices SET status='published',publish_at=COALESCE(?::timestamptz, now()),updated_by=?,updated_at=now() WHERE id=?", when, actor, id).Error; err != nil {
			return dbErr(err)
		}
		if out, err = r.get(tx, actor, id, true); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "notice.published", "notice", id, map[string]any{"publish_at": out.PublishAt})
	})
	return out, dbErr(err)
}

func (r *GormRepository) Archive(ctx context.Context, actor, id string) (Notice, error) {
	var out Notice
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockedStatus(tx, "notices", id)
		if err != nil {
			return err
		}
		if status == "archived" {
			return ErrConflict
		}
		if err := tx.Exec("UPDATE notices SET status='archived',updated_by=?,updated_at=now() WHERE id=?", actor, id).Error; err != nil {
			return err
		}
		if out, err = r.get(tx, actor, id, true); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "notice.archived", "notice", id, nil)
	})
	return out, dbErr(err)
}

func (r *GormRepository) Delete(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockedStatus(tx, "notices", id)
		if err != nil {
			return err
		}
		if status != "draft" {
			return ErrConflict
		}
		var attached []string
		if err := tx.Table("notice_attachments").Where("notice_id = ?", id).Pluck("file_id", &attached).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM notices WHERE id = ?", id).Error; err != nil {
			return err
		}
		if err := files.UnlinkTx(tx, attached); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "notice.deleted", "notice", id, nil)
	}))
}

func (r *GormRepository) AttachmentFile(ctx context.Context, actor, noticeID, fileID string, manager bool) (string, error) {
	if _, err := r.Get(ctx, actor, noticeID, manager); err != nil {
		return "", err
	}
	var n int64
	if err := r.db.WithContext(ctx).Table("notice_attachments").Where("notice_id = ? AND file_id = ?", noticeID, fileID).Count(&n).Error; err != nil {
		return "", err
	}
	if n == 0 {
		return "", ErrNotFound
	}
	return fileID, nil
}

const eventSelect = "e.id,e.title_en,e.title_bn,e.description_en,e.description_bn,e.kind,to_char(e.starts_on,'YYYY-MM-DD') AS starts_on,to_char(e.ends_on,'YYYY-MM-DD') AS ends_on," +
	"to_char(e.start_time,'HH24:MI') AS start_time,to_char(e.end_time,'HH24:MI') AS end_time,e.location,array_to_json(e.audience_roles)::text AS roles_json,e.status"

func hydrateEvents(db *gorm.DB, items []Event) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	index := map[string]int{}
	for i := range items {
		ids[i] = items[i].ID
		index[items[i].ID] = i
		items[i].Targets = []Target{}
		_ = json.Unmarshal([]byte(items[i].RolesJSON), &items[i].AudienceRoles)
	}
	var targets []struct {
		EventID string
		Target
	}
	if err := db.Table("school_event_targets t").Joins("JOIN academic_classes c ON c.id = t.class_id").Joins("LEFT JOIN academic_sections s ON s.id = t.section_id").
		Select("t.event_id,t.class_id,t.section_id,c.name AS class_name,COALESCE(s.name,'') AS section_name").Where("t.event_id IN ?", ids).Find(&targets).Error; err != nil {
		return err
	}
	for _, t := range targets {
		items[index[t.EventID]].Targets = append(items[index[t.EventID]].Targets, t.Target)
	}
	return nil
}

func (r *GormRepository) ListEvents(ctx context.Context, actor string, q Query, manager bool) ([]Event, error) {
	db := r.db.WithContext(ctx)
	base := db.Table("school_events e").Where("e.starts_on <= ?::date AND e.ends_on >= ?::date", q.To, q.From)
	if manager {
		if q.Status != "" {
			base = base.Where("e.status = ?", q.Status)
		}
	} else {
		base = base.Joins("JOIN users u ON u.id = ? AND u.is_active", actor).Where("e.status IN ('published','cancelled')").Where(AudiencePredicate("e", "school_event_targets", "event_id"))
	}
	out := []Event{}
	if err := base.Select(eventSelect).Order("e.starts_on, e.start_time NULLS FIRST, e.title_en").Limit(500).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, hydrateEvents(db, out)
}

func eventByID(tx *gorm.DB, id string) (Event, error) {
	var e Event
	if err := tx.Table("school_events e").Select(eventSelect).Where("e.id = ?", id).Take(&e).Error; err != nil {
		return Event{}, dbErr(err)
	}
	items := []Event{e}
	err := hydrateEvents(tx, items)
	return items[0], err
}

func (r *GormRepository) SaveEvent(ctx context.Context, actor, id string, in EventInput) (Event, error) {
	var out Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roles := pgTextArray(in.AudienceRoles)
		if id == "" {
			if err := tx.Raw("INSERT INTO school_events(title_en,title_bn,description_en,description_bn,kind,starts_on,ends_on,start_time,end_time,location,audience_roles,created_by,updated_by) VALUES (?,?,?,?,?,?::date,?::date,?::time,?::time,?,?::text[],?,?) RETURNING id",
				in.TitleEN, in.TitleBN, in.DescriptionEN, in.DescriptionBN, in.Kind, in.StartsOn, in.EndsOn, in.StartTime, in.EndTime, in.Location, roles, actor, actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			if _, err := lockedStatus(tx, "school_events", id); err != nil {
				return err
			}
			if err := tx.Exec("UPDATE school_events SET title_en=?,title_bn=?,description_en=?,description_bn=?,kind=?,starts_on=?::date,ends_on=?::date,start_time=?::time,end_time=?::time,location=?,audience_roles=?::text[],updated_by=?,updated_at=now() WHERE id=?",
				in.TitleEN, in.TitleBN, in.DescriptionEN, in.DescriptionBN, in.Kind, in.StartsOn, in.EndsOn, in.StartTime, in.EndTime, in.Location, roles, actor, id).Error; err != nil {
				return dbErr(err)
			}
		}
		if err := replaceTargets(tx, "school_event_targets", "event_id", id, in.Targets); err != nil {
			return err
		}
		var err error
		if out, err = eventByID(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "event.saved", "school_event", id, map[string]any{"kind": in.Kind, "starts_on": in.StartsOn})
	})
	return out, dbErr(err)
}

func (r *GormRepository) SetEventStatus(ctx context.Context, actor, id, status string) (Event, error) {
	var out Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := lockedStatus(tx, "school_events", id)
		if err != nil {
			return err
		}
		if current == status || (status == "cancelled" && current != "published") {
			return ErrConflict
		}
		if err := tx.Exec("UPDATE school_events SET status=?,updated_by=?,updated_at=now() WHERE id=?", status, actor, id).Error; err != nil {
			return err
		}
		if out, err = eventByID(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "event."+status, "school_event", id, nil)
	})
	return out, dbErr(err)
}

func (r *GormRepository) DeleteEvent(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockedStatus(tx, "school_events", id)
		if err != nil {
			return err
		}
		if status != "draft" {
			return ErrConflict
		}
		if err := tx.Exec("DELETE FROM school_events WHERE id = ?", id).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "event.deleted", "school_event", id, nil)
	}))
}
