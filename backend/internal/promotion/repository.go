package promotion

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func dbErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "FB001", "FB002":
			return ErrConflict
		case "23503", "23514", "22P02":
			return ErrInvalid
		}
	}
	return err
}

// lock serializes every promotion write: batches are rare and applies touch
// many students, so one lock is simpler and safe.
func lock(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext('flybirds.promotion'))").Error
}

const utc = `'YYYY-MM-DD"T"HH24:MI:SS"Z"'`

const batchSelect = `SELECT b.id, b.from_session_id, fs.name AS from_session_name, b.to_session_id, ts.name AS to_session_name,
       b.class_id, c.name AS class_name, c.next_class_id, nc.name AS next_class_name, c.is_final AS class_is_final, b.status, b.roll_policy, b.approved_sha256,
       coalesce(nullif(au.display_name, ''), au.email) AS approved_by, to_char(b.approved_at AT TIME ZONE 'UTC', ` + utc + `) AS approved_at,
       coalesce(nullif(pu.display_name, ''), pu.email) AS applied_by, to_char(b.applied_at AT TIME ZONE 'UTC', ` + utc + `) AS applied_at,
       coalesce(nullif(ru.display_name, ''), ru.email) AS reverted_by, to_char(b.reverted_at AT TIME ZONE 'UTC', ` + utc + `) AS reverted_at,
       b.revert_reason, coalesce(nullif(cu.display_name, ''), cu.email) AS created_by, to_char(b.created_at AT TIME ZONE 'UTC', ` + utc + `) AS created_at
  FROM promotion_batches b
  JOIN academic_sessions fs ON fs.id = b.from_session_id JOIN academic_sessions ts ON ts.id = b.to_session_id
  JOIN academic_classes c ON c.id = b.class_id LEFT JOIN academic_classes nc ON nc.id = c.next_class_id
  LEFT JOIN users au ON au.id = b.approved_by LEFT JOIN users pu ON pu.id = b.applied_by
  LEFT JOIN users ru ON ru.id = b.reverted_by JOIN users cu ON cu.id = b.created_by`

func (r *Repository) List(ctx context.Context, q Query) ([]Batch, error) {
	out := []Batch{}
	query := batchSelect + " WHERE (? = '' OR b.from_session_id::text = ? OR b.to_session_id::text = ?) AND (? = '' OR b.class_id::text = ?) ORDER BY b.created_at DESC, b.id LIMIT 200"
	if err := r.db.WithContext(ctx).Raw(query, q.SessionID, q.SessionID, q.SessionID, q.ClassID, q.ClassID).Scan(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Items, out[i].Blockers, out[i].Warnings, out[i].RevertBlockers, out[i].Counts = []Item{}, []Issue{}, []Issue{}, []Issue{}, map[string]int{}
	}
	return out, nil
}

func (r *Repository) Get(ctx context.Context, id string) (Batch, error) {
	return view(r.db.WithContext(ctx), id)
}

// view loads a batch with its items, blockers, warnings and the hash of the
// inputs an approval binds to.
func view(tx *gorm.DB, id string) (Batch, error) {
	var b Batch
	res := tx.Raw(batchSelect+" WHERE b.id = ?", id).Scan(&b)
	if res.Error != nil {
		return b, res.Error
	}
	if res.RowsAffected == 0 {
		return b, ErrNotFound
	}
	b.Items, b.Blockers, b.Warnings, b.RevertBlockers, b.Counts = []Item{}, []Issue{}, []Issue{}, []Issue{}, map[string]int{}
	if err := tx.Raw(`SELECT i.student_id, st.name_en AS student_name, st.admission_no, i.enrolment_id, i.source_section_id, ss.name AS source_section_name,
       i.source_roll_no, i.annual_version_id, i.annual_outcome, i.annual_class_position, i.recommendation, i.decision, i.reason,
       coalesce(nullif(du.display_name, ''), du.email) AS decided_by, i.target_class_id, tc.name AS target_class_name,
       i.target_section_id, tsec.name AS target_section_name, i.target_roll_no, i.applied_enrolment_id
  FROM promotion_items i JOIN students st ON st.id = i.student_id JOIN academic_sections ss ON ss.id = i.source_section_id
  LEFT JOIN users du ON du.id = i.decided_by LEFT JOIN academic_classes tc ON tc.id = i.target_class_id
  LEFT JOIN academic_sections tsec ON tsec.id = i.target_section_id
 WHERE i.batch_id = ?
 ORDER BY ss.display_order, ss.name, i.source_roll_no NULLS LAST, st.admission_no, i.student_id`, id).Scan(&b.Items).Error; err != nil {
		return b, err
	}
	for _, it := range b.Items {
		b.Counts["recommended_"+it.Recommendation]++
		if it.Decision != nil {
			b.Counts[*it.Decision]++
		} else {
			b.Counts["undecided"]++
		}
	}
	hash, err := inputsHash(b)
	if err != nil {
		return b, err
	}
	b.InputsSHA256 = hash
	if b.Status == "draft" || b.Status == "approved" {
		if err := issues(tx, &b); err != nil {
			return b, err
		}
		var logins int
		if err := tx.Raw(`SELECT count(*) FROM promotion_items i JOIN students s ON s.id = i.student_id JOIN users u ON u.id = s.user_id
 WHERE i.batch_id = ? AND i.decision IN ('withdraw', 'transfer') AND u.is_active`, b.ID).Row().Scan(&logins); err != nil {
			return b, err
		}
		b.Counts["logins_to_disable"] = logins
	}
	if b.Status == "applied" {
		if err := revertIssues(tx, &b); err != nil {
			return b, err
		}
	}
	return b, nil
}

// revertIssues fills why an applied batch cannot be reverted: a promoted or
// repeating student who has records in the new year (new_year_records), or
// any student who is no longer where the apply left them (student_moved).
func revertIssues(tx *gorm.DB, b *Batch) error {
	for _, it := range b.Items {
		if it.Decision == nil {
			continue
		}
		var placed, records bool
		switch *it.Decision {
		case Promote, Repeat:
			if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM students s JOIN student_enrolments e ON e.student_id = s.id AND e.state = 'current'
  WHERE s.id = ? AND s.session_id = ? AND s.status = 'active' AND e.id = ?), student_has_session_records(?, ?)`,
				it.StudentID, b.ToSessionID, it.AppliedEnrolmentID, it.StudentID, b.ToSessionID).Row().Scan(&placed, &records); err != nil {
				return err
			}
		default:
			if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM students s WHERE s.id = ? AND s.session_id = ? AND s.class_id = ? AND s.status = ?)`,
				it.StudentID, b.FromSessionID, b.ClassID, leaveStatus[*it.Decision]).Row().Scan(&placed); err != nil {
				return err
			}
		}
		switch {
		case !placed:
			b.RevertBlockers = append(b.RevertBlockers, Issue{Code: "student_moved", StudentID: it.StudentID})
		case records:
			b.RevertBlockers = append(b.RevertBlockers, Issue{Code: "new_year_records", StudentID: it.StudentID})
		}
	}
	return nil
}

// inputsHash covers everything an apply acts on: the sessions, class and its
// next class, each student's year, annual result, recommendation, decision,
// reason and placement.
func inputsHash(b Batch) (string, error) {
	type line struct {
		Student, Enrolment         string
		Annual, Outcome            *string
		Recommendation             string
		Decision                   *string
		Reason                     string
		TargetClass, TargetSection *string
		TargetRoll                 *int
	}
	lines := make([]line, 0, len(b.Items))
	for _, it := range b.Items {
		lines = append(lines, line{it.StudentID, it.EnrolmentID, it.AnnualVersionID, it.AnnualOutcome, it.Recommendation, it.Decision, it.Reason,
			it.TargetClassID, it.TargetSectionID, it.TargetRollNo})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Student < lines[j].Student })
	raw, err := json.Marshal(struct {
		Format                 int
		Batch, From, To, Class string
		Next                   *string
		Final                  bool
		Lines                  []line
	}{1, b.ID, b.FromSessionID, b.ToSessionID, b.ClassID, b.NextClassID, b.ClassIsFinal, lines})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// issues fills the blockers (approval and apply are refused while any
// remain) and warnings of an open batch.
func issues(tx *gorm.DB, b *Batch) error {
	block := func(code, student, detail string) { b.Blockers = append(b.Blockers, Issue{code, student, detail}) }
	warn := func(code, student, detail string) { b.Warnings = append(b.Warnings, Issue{code, student, detail}) }
	if b.NextClassID == nil && !b.ClassIsFinal {
		block("progression_unset", "", "set the class's next class or mark it final")
	}

	// Students whose year changed since the batch was made, students who
	// joined the class since, and annual results corrected since.
	changed := []string{}
	if err := tx.Raw(`SELECT i.student_id FROM promotion_items i JOIN promotion_batches b ON b.id = i.batch_id
  LEFT JOIN student_enrolments e ON e.id = i.enrolment_id
 WHERE i.batch_id = ? AND NOT coalesce(e.state = 'current' AND e.student_status = 'active' AND e.session_id = b.from_session_id AND e.class_id = b.class_id, false)`, b.ID).Scan(&changed).Error; err != nil {
		return err
	}
	for _, s := range changed {
		block("student_changed", s, "")
	}
	missing := []string{}
	if err := tx.Raw(`SELECT e.student_id FROM student_enrolments e
 WHERE e.session_id = ? AND e.class_id = ? AND e.state = 'current' AND e.student_status = 'active'
   AND NOT EXISTS (SELECT 1 FROM promotion_items i WHERE i.batch_id = ? AND i.student_id = e.student_id)`, b.FromSessionID, b.ClassID, b.ID).Scan(&missing).Error; err != nil {
		return err
	}
	for _, s := range missing {
		block("student_missing", s, "")
	}
	annual, err := annualResults(tx, b.FromSessionID, b.ClassID)
	if err != nil {
		return err
	}

	sectionOK := map[string]bool{}
	rows := []struct{ ID, ClassID string }{}
	if err := tx.Raw("SELECT id, class_id FROM academic_sections WHERE session_id = ? AND is_active", b.ToSessionID).Scan(&rows).Error; err != nil {
		return err
	}
	for _, s := range rows {
		sectionOK[s.ID+"|"+s.ClassID] = true
	}
	type key struct {
		section string
		roll    int
	}
	used := map[key]string{}
	sections := []string{}
	for _, it := range b.Items {
		var cur *string
		if a, ok := annual[it.StudentID]; ok {
			cur = &a.ID
		}
		if (cur == nil) != (it.AnnualVersionID == nil) || (cur != nil && *cur != *it.AnnualVersionID) {
			block("annual_changed", it.StudentID, "")
		}
		if it.Decision == nil {
			block("decision_required", it.StudentID, it.Recommendation)
			continue
		}
		if *it.Decision != it.Recommendation {
			warn("override", it.StudentID, it.Recommendation+" → "+*it.Decision)
		}
		if *it.Decision != Promote && *it.Decision != Repeat {
			continue
		}
		if *it.Decision == Promote && b.NextClassID == nil {
			block("no_next_class", it.StudentID, "")
			continue
		}
		if it.TargetSectionID == nil || it.TargetClassID == nil || !sectionOK[*it.TargetSectionID+"|"+*it.TargetClassID] {
			block("section_required", it.StudentID, "")
			continue
		}
		if it.TargetRollNo == nil {
			warn("roll_missing", it.StudentID, "")
			continue
		}
		k := key{*it.TargetSectionID, *it.TargetRollNo}
		if other, ok := used[k]; ok {
			block("roll_duplicate", it.StudentID, other)
		}
		used[k] = it.StudentID
		sections = append(sections, *it.TargetSectionID)
	}
	if len(sections) > 0 {
		taken := []struct {
			SectionID, StudentID string
			RollNo               int
		}{}
		if err := tx.Raw("SELECT section_id, student_id, roll_no FROM student_enrolments WHERE section_id IN ? AND roll_no IS NOT NULL", sections).Scan(&taken).Error; err != nil {
			return err
		}
		for _, t := range taken {
			if s, ok := used[key{t.SectionID, t.RollNo}]; ok && s != t.StudentID {
				block("roll_taken", s, t.StudentID)
			}
		}
	}

	var concessions int
	if err := tx.Raw(`SELECT count(*) FROM fee_concessions fc JOIN promotion_items i ON i.student_id = fc.student_id AND i.batch_id = ? WHERE fc.session_id = ?`, b.ID, b.FromSessionID).Row().Scan(&concessions); err != nil {
		return err
	}
	if concessions > 0 {
		warn("concessions_to_review", "", "fee concessions of the old session do not carry over; review them for the new session")
	}
	return nil
}

type annualRow struct {
	ID, StudentID, Outcome string
	ClassPosition          *int
}

// annualResults: the current published annual result of each student of a
// class in a session.
func annualResults(tx *gorm.DB, sessionID, classID string) (map[string]annualRow, error) {
	rows := []annualRow{}
	if err := tx.Raw(`SELECT id, student_id, outcome, class_position FROM result_versions
 WHERE scope = 'annual' AND scope_id = ? AND class_id = ? AND status = 'published'`, sessionID, classID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]annualRow{}
	for _, a := range rows {
		out[a.StudentID] = a
	}
	return out, nil
}

// recommend applies the approved rules: passed → promote (graduate from
// the final class); failed → repeat; withheld or no published annual
// result → blocked until someone decides with a reason.
func recommend(final bool, a *annualRow) string {
	switch {
	case a == nil || a.Outcome == "withheld":
		return Blocked
	case a.Outcome == "failed":
		return Repeat
	case final:
		return Graduate
	}
	return Promote
}

type batchRow struct {
	ID, FromSessionID, ToSessionID, ClassID, Status, RollPolicy string
	NextClassID                                                 *string
	IsFinal                                                     bool
}

func lockBatch(tx *gorm.DB, id string) (batchRow, error) {
	var b batchRow
	res := tx.Raw(`SELECT b.id, b.from_session_id, b.to_session_id, b.class_id, b.status, b.roll_policy, c.next_class_id, c.is_final
  FROM promotion_batches b JOIN academic_classes c ON c.id = b.class_id WHERE b.id = ? FOR UPDATE OF b`, id).Scan(&b)
	if res.Error != nil {
		return b, res.Error
	}
	if res.RowsAffected == 0 {
		return b, ErrNotFound
	}
	return b, nil
}

// syncItems makes the items match the class's current students and annual
// results. Decisions that followed the recommendation follow the new one;
// decisions made with a reason are kept.
func syncItems(tx *gorm.DB, b batchRow) error {
	students := []struct {
		EnrolmentID, StudentID, SectionID string
		RollNo                            *int
	}{}
	if err := tx.Raw(`SELECT id AS enrolment_id, student_id, section_id, roll_no FROM student_enrolments
 WHERE session_id = ? AND class_id = ? AND state = 'current' AND student_status = 'active'`, b.FromSessionID, b.ClassID).Scan(&students).Error; err != nil {
		return err
	}
	annual, err := annualResults(tx, b.FromSessionID, b.ClassID)
	if err != nil {
		return err
	}
	if err := tx.Exec(`DELETE FROM promotion_items i WHERE i.batch_id = ? AND NOT EXISTS (
  SELECT 1 FROM student_enrolments e WHERE e.student_id = i.student_id AND e.session_id = ? AND e.class_id = ? AND e.state = 'current' AND e.student_status = 'active')`,
		b.ID, b.FromSessionID, b.ClassID).Error; err != nil {
		return err
	}
	existing := []struct {
		StudentID, Recommendation, Reason string
		Decision                          *string
	}{}
	if err := tx.Raw("SELECT student_id, recommendation, decision, reason FROM promotion_items WHERE batch_id = ?", b.ID).Scan(&existing).Error; err != nil {
		return err
	}
	old := map[string]int{}
	for i, e := range existing {
		old[e.StudentID] = i
	}
	for _, st := range students {
		var a *annualRow
		var versionID, outcome *string
		var position *int
		if v, ok := annual[st.StudentID]; ok {
			a, versionID, outcome, position = &v, &v.ID, &v.Outcome, v.ClassPosition
		}
		rec := recommend(b.IsFinal, a)
		var decision *string
		if rec != Blocked {
			decision = &rec
		}
		if i, ok := old[st.StudentID]; ok {
			e := existing[i]
			if e.Decision != nil && (*e.Decision != e.Recommendation || e.Reason != "") {
				decision = e.Decision // a decision made with a reason stays
			}
			reason := e.Reason
			if decision != nil && *decision != rec && reason == "" {
				decision = nil
			}
			if err := tx.Exec(`UPDATE promotion_items SET enrolment_id = ?, source_section_id = ?, source_roll_no = ?, annual_version_id = ?, annual_outcome = ?,
       annual_class_position = ?, recommendation = ?, decision = ? WHERE batch_id = ? AND student_id = ?`,
				st.EnrolmentID, st.SectionID, st.RollNo, versionID, outcome, position, rec, decision, b.ID, st.StudentID).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Exec(`INSERT INTO promotion_items (batch_id, student_id, enrolment_id, source_section_id, source_roll_no, annual_version_id, annual_outcome,
       annual_class_position, recommendation, decision) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			b.ID, st.StudentID, st.EnrolmentID, st.SectionID, st.RollNo, versionID, outcome, position, rec, decision).Error; err != nil {
			return err
		}
	}
	return place(tx, b)
}

// place sets each moving student's target class (next class to promote,
// the same class to repeat), keeps a chosen section if it fits, otherwise
// takes the section with the same code in the new year, and numbers rolls
// by the batch's policy (keep, merit, name; manual leaves them).
func place(tx *gorm.DB, b batchRow) error {
	if err := tx.Exec(`UPDATE promotion_items SET target_class_id = CASE decision WHEN 'promote' THEN ?::uuid WHEN 'repeat' THEN ?::uuid END WHERE batch_id = ?`,
		b.NextClassID, b.ClassID, b.ID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE promotion_items i SET target_section_id = NULL, target_roll_no = NULL
 WHERE i.batch_id = ? AND (i.target_class_id IS NULL OR (i.target_section_id IS NOT NULL AND NOT EXISTS (
   SELECT 1 FROM academic_sections s WHERE s.id = i.target_section_id AND s.session_id = ? AND s.class_id = i.target_class_id AND s.is_active)))`,
		b.ID, b.ToSessionID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`UPDATE promotion_items i SET target_section_id = s.id
  FROM academic_sections src, academic_sections s
 WHERE i.batch_id = ? AND i.target_class_id IS NOT NULL AND i.target_section_id IS NULL AND src.id = i.source_section_id
   AND s.session_id = ? AND s.class_id = i.target_class_id AND s.is_active AND upper(s.code) = upper(src.code)`, b.ID, b.ToSessionID).Error; err != nil {
		return err
	}
	switch b.RollPolicy {
	case "manual":
		return nil
	case "keep":
		return tx.Exec("UPDATE promotion_items SET target_roll_no = CASE WHEN target_section_id IS NOT NULL THEN source_roll_no END WHERE batch_id = ?", b.ID).Error
	}
	order := "st.name_en, st.admission_no, i.student_id"
	if b.RollPolicy == "merit" {
		order = "i.annual_class_position NULLS LAST, st.name_en, st.admission_no, i.student_id"
	}
	items := []struct{ StudentID, SectionID string }{}
	if err := tx.Raw(`SELECT i.student_id, i.target_section_id AS section_id FROM promotion_items i JOIN students st ON st.id = i.student_id
 WHERE i.batch_id = ? AND i.target_section_id IS NOT NULL ORDER BY `+order, b.ID).Scan(&items).Error; err != nil {
		return err
	}
	if err := tx.Exec("UPDATE promotion_items SET target_roll_no = NULL WHERE batch_id = ?", b.ID).Error; err != nil {
		return err
	}
	taken := map[string]map[int]bool{}
	next := map[string]int{}
	for _, it := range items {
		if taken[it.SectionID] == nil {
			rolls := []int{}
			if err := tx.Raw("SELECT roll_no FROM student_enrolments WHERE section_id = ? AND roll_no IS NOT NULL", it.SectionID).Scan(&rolls).Error; err != nil {
				return err
			}
			taken[it.SectionID] = map[int]bool{}
			for _, n := range rolls {
				taken[it.SectionID][n] = true
			}
		}
		n := next[it.SectionID] + 1
		for taken[it.SectionID][n] {
			n++
		}
		next[it.SectionID] = n
		if err := tx.Exec("UPDATE promotion_items SET target_roll_no = ? WHERE batch_id = ? AND student_id = ?", n, b.ID, it.StudentID).Error; err != nil {
			return err
		}
	}
	return nil
}

// reopen returns an approved batch to draft after an edit.
func reopen(tx *gorm.DB, b batchRow) error {
	if b.Status != "approved" {
		return nil
	}
	return tx.Exec("UPDATE promotion_batches SET status = 'draft', approved_sha256 = NULL, approved_by = NULL, approved_at = NULL, updated_at = now() WHERE id = ?", b.ID).Error
}

func editable(b batchRow) error {
	if b.Status != "draft" && b.Status != "approved" {
		return ErrState
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, a Actor, in CreateInput) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		var later *bool
		if err := tx.Raw(`SELECT (SELECT start_date FROM academic_sessions WHERE id = ?) > (SELECT start_date FROM academic_sessions WHERE id = ?)`,
			in.ToSessionID, in.FromSessionID).Row().Scan(&later); err != nil {
			return err
		}
		if later == nil {
			return ErrNotFound
		}
		if !*later {
			return ErrInvalid
		}
		var id string
		if err := tx.Raw(`INSERT INTO promotion_batches (from_session_id, to_session_id, class_id, created_by) VALUES (?,?,?,?) RETURNING id`,
			in.FromSessionID, in.ToSessionID, in.ClassID, a.ID).Row().Scan(&id); err != nil {
			if errors.Is(dbErr(err), ErrConflict) {
				return ErrOpenBatch
			}
			return dbErr(err)
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		if b.NextClassID == nil && !b.IsFinal {
			return ErrProgressionUnset
		}
		if err := syncItems(tx, b); err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "promotion.batch_created", "promotion_batch", id, map[string]any{
			"from_session_id": in.FromSessionID, "to_session_id": in.ToSessionID, "class_id": in.ClassID, "students": len(out.Items)})
	})
	return out, dbErr(err)
}

func (r *Repository) UpdateItem(ctx context.Context, a Actor, id, studentID string, in ItemInput) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		if err := editable(b); err != nil {
			return err
		}
		var it struct {
			Recommendation, Reason string
			Decision               *string
		}
		res := tx.Raw("SELECT recommendation, decision, reason FROM promotion_items WHERE batch_id = ? AND student_id = ? FOR UPDATE", id, studentID).Scan(&it)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		decision, reason := it.Decision, it.Reason
		if in.Decision != nil {
			decision = in.Decision
		}
		if in.Reason != nil {
			reason = *in.Reason
		}
		if decision != nil && *decision != it.Recommendation && reason == "" {
			return ErrReasonRequired
		}
		if err := tx.Exec("UPDATE promotion_items SET decision = ?, reason = ?, decided_by = ?, decided_at = now() WHERE batch_id = ? AND student_id = ?",
			decision, reason, a.ID, id, studentID).Error; err != nil {
			return dbErr(err)
		}
		if in.TargetRollNo != nil {
			b.RollPolicy = "manual"
			if err := tx.Exec("UPDATE promotion_batches SET roll_policy = 'manual' WHERE id = ?", id).Error; err != nil {
				return err
			}
		}
		if err := place(tx, b); err != nil {
			return err
		}
		if in.TargetSectionID != nil {
			res := tx.Exec(`UPDATE promotion_items i SET target_section_id = s.id FROM academic_sections s
 WHERE i.batch_id = ? AND i.student_id = ? AND s.id = ? AND s.session_id = ? AND s.class_id = i.target_class_id AND s.is_active`,
				id, studentID, *in.TargetSectionID, b.ToSessionID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrInvalid
			}
			if b.RollPolicy != "manual" {
				if err := place(tx, b); err != nil {
					return err
				}
			}
		}
		if in.TargetRollNo != nil {
			if err := tx.Exec("UPDATE promotion_items SET target_roll_no = ? WHERE batch_id = ? AND student_id = ? AND target_section_id IS NOT NULL",
				*in.TargetRollNo, id, studentID).Error; err != nil {
				return err
			}
		}
		if err := reopen(tx, b); err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "promotion.item_updated", "promotion_batch", id, map[string]any{
			"student_id": studentID, "recommendation": it.Recommendation, "decision": decision, "reason": reason,
			"target_section_id": in.TargetSectionID, "target_roll_no": in.TargetRollNo, "approval_cleared": b.Status == "approved"})
	})
	return out, dbErr(err)
}

func (r *Repository) Arrange(ctx context.Context, a Actor, id, policy string) (Batch, error) {
	return r.edit(ctx, a, id, "promotion.rolls_arranged", map[string]any{"roll_policy": policy}, func(tx *gorm.DB, b *batchRow) error {
		b.RollPolicy = policy
		if err := tx.Exec("UPDATE promotion_batches SET roll_policy = ?, updated_at = now() WHERE id = ?", policy, b.ID).Error; err != nil {
			return err
		}
		return place(tx, *b)
	})
}

func (r *Repository) Refresh(ctx context.Context, a Actor, id string) (Batch, error) {
	return r.edit(ctx, a, id, "promotion.batch_refreshed", nil, func(tx *gorm.DB, b *batchRow) error {
		if b.NextClassID == nil && !b.IsFinal {
			return ErrProgressionUnset
		}
		return syncItems(tx, *b)
	})
}

func (r *Repository) edit(ctx context.Context, a Actor, id, action string, meta map[string]any, fn func(*gorm.DB, *batchRow) error) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		if err := editable(b); err != nil {
			return err
		}
		if err := fn(tx, &b); err != nil {
			return err
		}
		if err := reopen(tx, b); err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, action, "promotion_batch", id, meta)
	})
	return out, dbErr(err)
}

func (r *Repository) Discard(ctx context.Context, a Actor, id string) (Batch, error) {
	return r.edit(ctx, a, id, "promotion.batch_discarded", nil, func(tx *gorm.DB, b *batchRow) error {
		b.Status = "discarded"
		return tx.Exec("UPDATE promotion_batches SET status = 'discarded', updated_at = now() WHERE id = ?", b.ID).Error
	})
}

func (r *Repository) Approve(ctx context.Context, a Actor, id, hash string) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		v, err := view(tx, id)
		if err != nil {
			return err
		}
		if b.Status == "approved" && v.ApprovedSHA256 != nil && *v.ApprovedSHA256 == hash && v.InputsSHA256 == hash {
			out = v // approving the same inputs again changes nothing
			return nil
		}
		if b.Status != "draft" {
			return ErrState
		}
		if len(v.Blockers) > 0 {
			return ErrBlocked
		}
		if v.InputsSHA256 != hash {
			return ErrStale
		}
		if err := tx.Exec("UPDATE promotion_batches SET status = 'approved', approved_sha256 = ?, approved_by = ?, approved_at = now(), updated_at = now() WHERE id = ?",
			hash, a.ID, id).Error; err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "promotion.batch_approved", "promotion_batch", id, map[string]any{"inputs_sha256": hash, "students": len(v.Items), "counts": v.Counts})
	})
	return out, dbErr(err)
}

// settings for the enrolment triggers, for the rest of the transaction.
// completed marks a withdrawal or transfer as leaving after the year.
func settings(tx *gorm.DB, actor, outcome, source, revert, completed string) error {
	return tx.Exec(`SELECT set_config('flybirds.actor_id', ?, true), set_config('flybirds.enrolment_outcome', ?, true),
       set_config('flybirds.enrolment_source', ?, true), set_config('flybirds.enrolment_revert', ?, true),
       set_config('flybirds.enrolment_completed', ?, true)`, actor, outcome, source, revert, completed).Error
}

var leaveStatus = map[string]string{Graduate: "graduated", Withdraw: "inactive", Transfer: "transferred"}

// Apply moves every student of an approved batch in one transaction. The
// approval must still match the inputs. Applying an applied batch again
// changes nothing; a second administrator applying at the same time waits
// for the first and then gets the same result.
func (r *Repository) Apply(ctx context.Context, a Actor, id string) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		if b.Status == "applied" {
			out, err = view(tx, id)
			return err
		}
		if b.Status != "approved" {
			return ErrState
		}
		v, err := view(tx, id)
		if err != nil {
			return err
		}
		if len(v.Blockers) > 0 {
			return ErrBlocked
		}
		if v.ApprovedSHA256 == nil || *v.ApprovedSHA256 != v.InputsSHA256 {
			return ErrStale
		}
		counts := map[string]int{}
		for _, it := range v.Items {
			d := *it.Decision
			counts[d]++
			var res *gorm.DB
			switch d {
			case Promote, Repeat:
				outcome := map[string]string{Promote: "promoted", Repeat: "repeated"}[d]
				if err := settings(tx, a.ID, outcome, "promotion", "", ""); err != nil {
					return err
				}
				res = tx.Exec(`UPDATE students SET session_id = ?, class_id = ?, section_id = ?, roll_no = ?, updated_at = now()
 WHERE id = ? AND session_id = ? AND class_id = ? AND status = 'active'`,
					b.ToSessionID, *it.TargetClassID, *it.TargetSectionID, it.TargetRollNo, it.StudentID, b.FromSessionID, b.ClassID)
			default:
				// Leaving through the batch is leaving after the year: the
				// student stays on that year's roll.
				if err := settings(tx, a.ID, "", "", "", "on"); err != nil {
					return err
				}
				res = tx.Exec(`UPDATE students SET status = ?, updated_at = now() WHERE id = ? AND session_id = ? AND class_id = ? AND status = 'active'`,
					leaveStatus[d], it.StudentID, b.FromSessionID, b.ClassID)
			}
			if res.Error != nil {
				return dbErr(res.Error)
			}
			if res.RowsAffected != 1 {
				return ErrConflict
			}
			if d == Promote || d == Repeat {
				if err := tx.Exec(`UPDATE promotion_items SET applied_enrolment_id = (SELECT id FROM student_enrolments WHERE student_id = ? AND session_id = ?)
 WHERE batch_id = ? AND student_id = ?`, it.StudentID, b.ToSessionID, id, it.StudentID).Error; err != nil {
					return err
				}
			}
		}
		if err := settings(tx, a.ID, "", "", "", ""); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE promotion_batches SET status = 'applied', applied_by = ?, applied_at = now(), updated_at = now() WHERE id = ?", a.ID, id).Error; err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "promotion.batch_applied", "promotion_batch", id, map[string]any{
			"inputs_sha256": v.InputsSHA256, "students": len(v.Items), "counts": counts, "to_session_id": b.ToSessionID})
	})
	return out, dbErr(err)
}

// Revert undoes an applied batch while the new year has nothing recorded
// for its students and nobody has been moved since. Promoted and repeating
// students return to their old year (the new enrolment is removed); students
// who graduated or left are readmitted to it (a suspended login returns).
func (r *Repository) Revert(ctx context.Context, a Actor, id, reason string) (Batch, error) {
	var out Batch
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		b, err := lockBatch(tx, id)
		if err != nil {
			return err
		}
		if b.Status != "applied" {
			return ErrState
		}
		v, err := view(tx, id)
		if err != nil {
			return err
		}
		if len(v.RevertBlockers) > 0 {
			return ErrRevertRefused
		}
		for _, it := range v.Items {
			var res *gorm.DB
			switch *it.Decision {
			case Promote, Repeat:
				if err := settings(tx, a.ID, "", "", "on", ""); err != nil {
					return err
				}
				res = tx.Exec("UPDATE students SET session_id = ?, class_id = ?, section_id = ?, roll_no = ?, updated_at = now() WHERE id = ?",
					b.FromSessionID, b.ClassID, it.SourceSectionID, it.SourceRollNo, it.StudentID)
			default:
				if err := settings(tx, a.ID, "", "", "", ""); err != nil {
					return err
				}
				res = tx.Exec("UPDATE students SET status = 'active', updated_at = now() WHERE id = ?", it.StudentID)
			}
			if res.Error != nil {
				return dbErr(res.Error)
			}
		}
		if err := settings(tx, a.ID, "", "", "", ""); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE promotion_batches SET status = 'reverted', reverted_by = ?, reverted_at = now(), revert_reason = ?, updated_at = now() WHERE id = ?",
			a.ID, reason, id).Error; err != nil {
			return err
		}
		if out, err = view(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "promotion.batch_reverted", "promotion_batch", id, map[string]any{"reason": reason, "students": len(v.Items)})
	})
	return out, dbErr(err)
}

func (r *Repository) Progression(ctx context.Context) ([]ClassProgression, error) {
	out := []ClassProgression{}
	err := r.db.WithContext(ctx).Raw(`SELECT c.id AS class_id, c.name AS class_name, c.display_order, c.is_active,
       CASE WHEN c.next_class_id IS NOT NULL THEN 'next' WHEN c.is_final THEN 'final' ELSE 'unset' END AS setting,
       c.next_class_id, n.name AS next_class_name, c.is_final, sg.id AS suggested_class_id, sg.name AS suggested_class_name
  FROM academic_classes c LEFT JOIN academic_classes n ON n.id = c.next_class_id
  LEFT JOIN LATERAL (SELECT h.id, h.name FROM academic_classes h
                      WHERE c.next_class_id IS NULL AND NOT c.is_final AND h.is_active AND h.id <> c.id AND h.display_order > c.display_order
                      ORDER BY h.display_order, h.name, h.id LIMIT 1) sg ON true
 ORDER BY c.display_order, c.name, c.id`).Scan(&out).Error
	return out, err
}

func (r *Repository) SetProgression(ctx context.Context, a Actor, classID string, next *string, final bool) ([]ClassProgression, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		var before *string
		var wasFinal bool
		if err := tx.Raw("SELECT next_class_id, is_final FROM academic_classes WHERE id = ? FOR UPDATE", classID).Row().Scan(&before, &wasFinal); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if next != nil {
			// The next class must exist and must not lead back to this class.
			var cycle *bool
			if err := tx.Raw(`WITH RECURSIVE chain(id, depth) AS (
  SELECT id, 0 FROM academic_classes WHERE id = ?
  UNION ALL SELECT c.next_class_id, chain.depth + 1 FROM chain JOIN academic_classes c ON c.id = chain.id WHERE c.next_class_id IS NOT NULL AND chain.depth < 1000)
SELECT bool_or(id = ?) FROM chain`, *next, classID).Row().Scan(&cycle); err != nil {
				return err
			}
			if cycle == nil {
				return ErrNotFound
			}
			if *cycle {
				return ErrInvalid
			}
		}
		if err := tx.Exec("UPDATE academic_classes SET next_class_id = ?, is_final = ?, updated_at = now() WHERE id = ?", next, final, classID).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &a.ID, "academic.class_progression_saved", "academic_class", classID, map[string]any{
			"before_next_class_id": before, "before_final": wasFinal, "next_class_id": next, "final": final})
	})
	if err != nil {
		return nil, dbErr(err)
	}
	return r.Progression(ctx)
}
