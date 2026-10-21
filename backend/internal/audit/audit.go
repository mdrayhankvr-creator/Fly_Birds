package audit

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrInvalid = errors.New("invalid audit query")
var safeFilter = regexp.MustCompile(`^[a-z][a-z0-9_.:]*$`)

// Event contains only explicit, non-secret metadata supplied by repositories.
// No request body or credential is ever serialized into an audit record.
type Event struct {
	ID          string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	ActorUserID *string         `gorm:"type:uuid" json:"actor_user_id"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	Outcome     string          `json:"outcome"`
	Metadata    json.RawMessage `gorm:"type:jsonb" json:"metadata"`
}

func (Event) TableName() string { return "audit_events" }

func Record(tx *gorm.DB, actor *string, action, targetType, targetID string, metadata map[string]any) error {
	if !safeFilter.MatchString(action) || !safeFilter.MatchString(targetType) || targetID == "" {
		return ErrInvalid
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return tx.Create(&Event{ActorUserID: actor, Action: action, TargetType: targetType, TargetID: targetID, Outcome: "success", Metadata: body}).Error
}

// RecordDenied logs a refused administrative request. Only the method, route
// pattern and the reason are kept; request bodies are never recorded.
// Entry is one event for RecordMany.
type Entry struct {
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// RecordMany records several successful events by one actor in one
// statement, with the same validation as Record.
func RecordMany(tx *gorm.DB, actor *string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	events := make([]Event, 0, len(entries))
	for _, e := range entries {
		if !safeFilter.MatchString(e.Action) || !safeFilter.MatchString(e.TargetType) || e.TargetID == "" {
			return ErrInvalid
		}
		if e.Metadata == nil {
			e.Metadata = map[string]any{}
		}
		body, err := json.Marshal(e.Metadata)
		if err != nil {
			return err
		}
		events = append(events, Event{ActorUserID: actor, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Outcome: "success", Metadata: body})
	}
	return tx.CreateInBatches(&events, 500).Error
}

func RecordDenied(tx *gorm.DB, actor *string, method, route, reason string) error {
	body, err := json.Marshal(map[string]any{"method": method, "route": route, "reason": reason})
	if err != nil {
		return err
	}
	return tx.Create(&Event{ActorUserID: actor, Action: "admin.access_denied", TargetType: "route", TargetID: method + " " + route, Outcome: "denied", Metadata: body}).Error
}

type Query struct {
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	Outcome    string
	From       string
	To         string
	Search     string
	Page       int
	PageSize   int
	Newest     bool
}

// Item is an audit event with the actor's name for the viewer.
type Item struct {
	Event
	ActorName  string `json:"actor_name"`
	ActorEmail string `json:"actor_email"`
	ActorRole  string `json:"actor_role"`
}
type ActionCount struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}
type Page struct {
	Items    []Item `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, query Query) (Page, error) {
	base := r.db.WithContext(ctx).Table("audit_events AS e").Joins("LEFT JOIN users AS u ON u.id = e.actor_user_id")
	if query.Action != "" {
		base = base.Where("e.action = ?", query.Action)
	}
	if query.TargetType != "" {
		base = base.Where("e.target_type = ?", query.TargetType)
	}
	if query.TargetID != "" {
		base = base.Where("e.target_id = ?", query.TargetID)
	}
	if query.Actor != "" {
		base = base.Where("e.actor_user_id = ?", query.Actor)
	}
	if query.Outcome != "" {
		base = base.Where("e.outcome = ?", query.Outcome)
	}
	if query.From != "" {
		base = base.Where("(e.created_at AT TIME ZONE 'Asia/Dhaka')::date >= ?::date", query.From)
	}
	if query.To != "" {
		base = base.Where("(e.created_at AT TIME ZONE 'Asia/Dhaka')::date <= ?::date", query.To)
	}
	if query.Search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(query.Search)) + "%"
		base = base.Where("(lower(e.action) LIKE ? OR lower(e.target_id) LIKE ? OR lower(COALESCE(u.email, '')) LIKE ? OR lower(COALESCE(u.display_name, '')) LIKE ?)", pattern, pattern, pattern, pattern)
	}
	var page Page
	page.Items = []Item{}
	page.Page, page.PageSize = query.Page, query.PageSize
	if err := base.Count(&page.Total).Error; err != nil {
		return Page{}, err
	}
	order := "e.created_at DESC, e.id DESC"
	if !query.Newest {
		order = "e.created_at ASC, e.id ASC"
	}
	err := base.Select("e.*, COALESCE(NULLIF(u.display_name, ''), u.email, '') AS actor_name, COALESCE(u.email, '') AS actor_email, COALESCE(u.role, '') AS actor_role").
		Order(order).Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize).Scan(&page.Items).Error
	return page, err
}

// Actions lists recorded action names with counts for the viewer's filter.
func (r *Repository) Actions(ctx context.Context) ([]ActionCount, error) {
	out := []ActionCount{}
	err := r.db.WithContext(ctx).Table("audit_events").Select("action, count(*) AS count").Group("action").Order("action").Limit(300).Scan(&out).Error
	return out, err
}

// Denied records a refused administrative request.
func (r *Repository) Denied(ctx context.Context, actor *string, method, route, reason string) error {
	return RecordDenied(r.db.WithContext(ctx), actor, method, route, reason)
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
