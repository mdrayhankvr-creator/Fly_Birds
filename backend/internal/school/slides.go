package school

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // decode PNG uploads; they are stored as JPEG
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Landing-page slideshow limits. Browsers shrink photos to about 1600px wide
// before upload; the server re-checks every limit and re-encodes the image.
const (
	MaxSlides         = 8
	MaxSlideUpload    = 3 << 20 // bytes accepted from the client
	MaxSlideStored    = 1 << 20 // bytes after re-encoding
	MaxSlideDimension = 2400
	MinSlideDimension = 320
)

// ErrSlideLimit is returned when the slideshow already holds MaxSlides photos.
var ErrSlideLimit = errors.New("slideshow is full")

// Slide is a slideshow photo without its bytes.
type Slide struct {
	ID        string `json:"id"`
	Position  int    `json:"position"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Version   string `json:"version"`
	CaptionEN string `json:"caption_en"`
	CaptionBN string `json:"caption_bn"`
}

// SlideImage is a stored slideshow photo.
type SlideImage struct {
	ContentType string
	Content     []byte
	Version     string
}

type SlideCaption struct {
	CaptionEN string `json:"caption_en"`
	CaptionBN string `json:"caption_bn"`
}

// SlideRepository stores slideshow photos. GormRepository implements it.
type SlideRepository interface {
	Slides(context.Context) ([]Slide, error)
	SlideImage(context.Context, string) (SlideImage, error)
	AddSlide(context.Context, string, SlideImage, int, int) (Slide, error)
	UpdateSlideCaption(context.Context, string, string, SlideCaption) (Slide, error)
	ReorderSlides(context.Context, string, []string) ([]Slide, error)
	DeleteSlide(context.Context, string, string) error
}

func (s *Service) slides() (SlideRepository, error) {
	repo, ok := s.repo.(SlideRepository)
	if !ok {
		return nil, ErrNotFound
	}
	return repo, nil
}

func (s *Service) Slides(ctx context.Context) ([]Slide, error) {
	repo, err := s.slides()
	if err != nil {
		return nil, err
	}
	return repo.Slides(ctx)
}

func (s *Service) SlideImage(ctx context.Context, id string) (SlideImage, error) {
	repo, err := s.slides()
	if err != nil {
		return SlideImage{}, err
	}
	if !actorID.MatchString(id) {
		return SlideImage{}, ErrNotFound
	}
	return repo.SlideImage(ctx, id)
}

// AddSlide checks an uploaded JPEG or PNG and stores it re-encoded as JPEG.
// Re-encoding drops embedded metadata such as camera location.
func (s *Service) AddSlide(ctx context.Context, actor, contentType string, data []byte) (Slide, error) {
	repo, err := s.slides()
	if err != nil {
		return Slide{}, err
	}
	if !actorID.MatchString(actor) || len(data) < 1 || len(data) > MaxSlideUpload {
		return Slide{}, ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format == "jpeg" && contentType != "image/jpeg") || (format == "png" && contentType != "image/png") || (format != "jpeg" && format != "png") {
		return Slide{}, ErrInvalid
	}
	if config.Width < MinSlideDimension || config.Height < MinSlideDimension/2 || config.Width > MaxSlideDimension || config.Height > MaxSlideDimension {
		return Slide{}, ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Slide{}, ErrInvalid
	}
	// Flatten any transparency onto white so PNG cut-outs do not turn black.
	bounds := decoded.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, bounds, decoded, bounds.Min, draw.Over)
	var clean bytes.Buffer
	if err := jpeg.Encode(&clean, canvas, &jpeg.Options{Quality: 82}); err != nil || clean.Len() > MaxSlideStored {
		return Slide{}, ErrInvalid
	}
	sum := sha256.Sum256(clean.Bytes())
	stored := SlideImage{ContentType: "image/jpeg", Content: clean.Bytes(), Version: hex.EncodeToString(sum[:8])}
	return repo.AddSlide(ctx, actor, stored, bounds.Dx(), bounds.Dy())
}

func (s *Service) UpdateSlideCaption(ctx context.Context, actor, id string, caption SlideCaption) (Slide, error) {
	repo, err := s.slides()
	if err != nil {
		return Slide{}, err
	}
	caption.CaptionEN, caption.CaptionBN = strings.TrimSpace(caption.CaptionEN), strings.TrimSpace(caption.CaptionBN)
	if !actorID.MatchString(actor) || !actorID.MatchString(id) || !validLine(caption.CaptionEN, 140) || !validLine(caption.CaptionBN, 140) {
		return Slide{}, ErrInvalid
	}
	return repo.UpdateSlideCaption(ctx, actor, id, caption)
}

// ReorderSlides takes every slide ID once, in the new order.
func (s *Service) ReorderSlides(ctx context.Context, actor string, ids []string) ([]Slide, error) {
	repo, err := s.slides()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !actorID.MatchString(id) || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	if !actorID.MatchString(actor) || len(ids) > MaxSlides {
		return nil, ErrInvalid
	}
	return repo.ReorderSlides(ctx, actor, ids)
}

func (s *Service) DeleteSlide(ctx context.Context, actor, id string) error {
	repo, err := s.slides()
	if err != nil {
		return err
	}
	if !actorID.MatchString(actor) || !actorID.MatchString(id) {
		return ErrInvalid
	}
	return repo.DeleteSlide(ctx, actor, id)
}

const slideColumns = "id::text AS id, position, width, height, version, caption_en, caption_bn"

func slidesIn(tx *gorm.DB) ([]Slide, error) {
	out := []Slide{}
	err := tx.Table("school_slides").Select(slideColumns).Order("position").Scan(&out).Error
	return out, err
}

func (r *GormRepository) Slides(ctx context.Context) ([]Slide, error) {
	return slidesIn(r.db.WithContext(ctx))
}

func (r *GormRepository) SlideImage(ctx context.Context, id string) (SlideImage, error) {
	var out SlideImage
	result := r.db.WithContext(ctx).Table("school_slides").Select("content_type, content, version").Where("id = ?", id).Scan(&out)
	if result.Error != nil {
		return SlideImage{}, result.Error
	}
	if result.RowsAffected == 0 {
		return SlideImage{}, ErrNotFound
	}
	return out, nil
}

// slideWrite runs fn as an administrator holding the school lock, so
// concurrent uploads and reorders cannot race past the limit or positions.
func (r *GormRepository) slideWrite(ctx context.Context, actor string, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "school_settings:update"); err != nil {
			return err
		}
		return fn(tx)
	})
}

func (r *GormRepository) AddSlide(ctx context.Context, actor string, img SlideImage, width, height int) (Slide, error) {
	var out Slide
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("school_slides").Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxSlides {
			return ErrSlideLimit
		}
		var id string
		if err := tx.Raw("INSERT INTO school_slides (position, content_type, content, width, height, version, created_by) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id::text",
			count, img.ContentType, img.Content, width, height, img.Version, actor).Scan(&id).Error; err != nil {
			return err
		}
		if err := tx.Table("school_slides").Select(slideColumns).Where("id = ?", id).Scan(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.slide_added", "school_slide", id, map[string]any{"width": width, "height": height, "bytes": len(img.Content)})
	})
	return out, err
}

func (r *GormRepository) UpdateSlideCaption(ctx context.Context, actor, id string, caption SlideCaption) (Slide, error) {
	var out Slide
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		result := tx.Exec("UPDATE school_slides SET caption_en = ?, caption_bn = ?, updated_at = ? WHERE id = ?", caption.CaptionEN, caption.CaptionBN, time.Now().UTC(), id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Table("school_slides").Select(slideColumns).Where("id = ?", id).Scan(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.slide_caption_updated", "school_slide", id, nil)
	})
	return out, err
}

func (r *GormRepository) ReorderSlides(ctx context.Context, actor string, ids []string) ([]Slide, error) {
	var out []Slide
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		var current []string
		if err := tx.Table("school_slides").Pluck("id::text", &current).Error; err != nil {
			return err
		}
		if len(current) != len(ids) {
			return ErrInvalid
		}
		known := map[string]bool{}
		for _, id := range current {
			known[id] = true
		}
		for position, id := range ids {
			if !known[id] {
				return ErrInvalid
			}
			if err := tx.Exec("UPDATE school_slides SET position = ?, updated_at = now() WHERE id = ?", position, id).Error; err != nil {
				return err
			}
		}
		var err error
		if out, err = slidesIn(tx); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.slides_reordered", "school", "1", map[string]any{"order": ids})
	})
	return out, err
}

func (r *GormRepository) DeleteSlide(ctx context.Context, actor, id string) error {
	return r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		result := tx.Exec("DELETE FROM school_slides WHERE id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		// Close the gap so positions stay 0..n-1.
		if err := tx.Exec("UPDATE school_slides AS s SET position = o.rn - 1 FROM (SELECT id, row_number() OVER (ORDER BY position) AS rn FROM school_slides) AS o WHERE s.id = o.id").Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.slide_removed", "school_slide", id, nil)
	})
}
