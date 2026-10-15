package audit

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
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

type Query struct {
	Actor      string
	Action     string
	TargetType string
	Page       int
	PageSize   int
	Newest     bool
}
type Page struct {
	Items    []Event `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, query Query) (Page, error) {
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 ||
		(query.Action != "" && !safeFilter.MatchString(query.Action)) ||
		(query.TargetType != "" && !safeFilter.MatchString(query.TargetType)) ||
		(query.Actor != "" && !uuid.MatchString(query.Actor)) {
		return Page{}, ErrInvalid
	}
	base := r.db.WithContext(ctx).Model(&Event{})
	if query.Action != "" {
		base = base.Where("action = ?", query.Action)
	}
	if query.TargetType != "" {
		base = base.Where("target_type = ?", query.TargetType)
	}
	if query.Actor != "" {
		base = base.Where("actor_user_id = ?", query.Actor)
	}
	var page Page
	page.Items = []Event{}
	page.Page, page.PageSize = query.Page, query.PageSize
	if err := base.Count(&page.Total).Error; err != nil {
		return Page{}, err
	}
	order := "created_at DESC, id DESC"
	if !query.Newest {
		order = "created_at ASC, id ASC"
	}
	err := base.Order(order).Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize).Find(&page.Items).Error
	return page, err
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
