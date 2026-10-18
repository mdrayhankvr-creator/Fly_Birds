package files

import (
	"context"
	"errors"
	"io"
	"regexp"
	"time"

	"gorm.io/gorm"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// MaxPendingUploads bounds unlinked uploads per user so abandoned forms cannot
// fill the disk before cleanup runs.
const MaxPendingUploads = 30

// AbandonAfter is how long an upload may stay unattached before cleanup.
const AbandonAfter = 24 * time.Hour

type Meta struct {
	ID          string `json:"id"`
	Purpose     string `json:"purpose"`
	Name        string `json:"name" gorm:"column:original_name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size" gorm:"column:size_bytes"`
	StorageKey  string `json:"-"`
	UploadedBy  string `json:"-"`
}

type Service struct {
	db    *gorm.DB
	store Store
}

func NewService(db *gorm.DB, store Store) *Service { return &Service{db: db, store: store} }

func ValidPurpose(p string) bool { return p == "notice" || p == "diary" || p == "homework" }

// Upload validates and stores a file for later attachment by its uploader.
func (s *Service) Upload(ctx context.Context, actor, purpose, name string, data []byte) (Meta, error) {
	if !uuidPattern.MatchString(actor) || !ValidPurpose(purpose) {
		return Meta{}, ErrInvalid
	}
	clean, err := Validate(name, data)
	if err != nil {
		return Meta{}, err
	}
	var pending int64
	if err := s.db.WithContext(ctx).Table("file_objects").Where("uploaded_by=? AND linked_at IS NULL", actor).Count(&pending).Error; err != nil {
		return Meta{}, err
	}
	if pending >= MaxPendingUploads {
		return Meta{}, ErrTooLarge
	}
	key, err := NewKey()
	if err != nil {
		return Meta{}, err
	}
	if err := s.store.Put(ctx, key, clean.Data); err != nil {
		return Meta{}, err
	}
	out := Meta{Purpose: purpose, Name: clean.Name, ContentType: clean.ContentType, Size: len(clean.Data), StorageKey: key, UploadedBy: actor}
	err = s.db.WithContext(ctx).Raw("INSERT INTO file_objects(storage_key,purpose,original_name,content_type,size_bytes,sha256,uploaded_by) VALUES (?,?,?,?,?,?,?) RETURNING id", key, purpose, clean.Name, clean.ContentType, len(clean.Data), clean.SHA256, actor).Row().Scan(&out.ID)
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return Meta{}, err
	}
	return out, nil
}

// Open returns a linked file. Callers must authorize access to the owning
// notice, diary or submission first.
func (s *Service) Open(ctx context.Context, id string) (Meta, io.ReadCloser, error) {
	if !uuidPattern.MatchString(id) {
		return Meta{}, nil, ErrNotFound
	}
	var m Meta
	err := s.db.WithContext(ctx).Table("file_objects").Select("id,purpose,original_name,content_type,size_bytes,storage_key,uploaded_by").Where("id=?", id).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Meta{}, nil, ErrNotFound
	}
	if err != nil {
		return Meta{}, nil, err
	}
	r, err := s.store.Open(ctx, m.StorageKey)
	return m, r, err
}

// LinkTx claims the actor's own unattached uploads of the given purpose inside
// the caller's transaction. Any other file id makes the whole request invalid.
func LinkTx(tx *gorm.DB, actor, purpose string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			return ErrInvalid
		}
	}
	res := tx.Exec("UPDATE file_objects SET linked_at=now() WHERE id IN ? AND uploaded_by=? AND purpose=? AND linked_at IS NULL", ids, actor, purpose)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != int64(len(ids)) {
		return ErrInvalid
	}
	return nil
}

// UnlinkTx releases files removed from an entity; cleanup deletes them later.
func UnlinkTx(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Exec("UPDATE file_objects SET linked_at=NULL, created_at=now()-interval '1 day' WHERE id IN ?", ids).Error
}

// Cleanup deletes uploads that were never attached or were removed.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	var keys []string
	err := s.db.WithContext(ctx).Raw("DELETE FROM file_objects WHERE linked_at IS NULL AND created_at < now() - ?::interval RETURNING storage_key", AbandonAfter.String()).Scan(&keys).Error
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		_ = s.store.Delete(ctx, key)
	}
	return len(keys), nil
}
