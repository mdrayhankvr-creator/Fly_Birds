package staff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Signatures (marksheet v2): each member of staff uploads their own
// signature, printed on documents (a class teacher's on marksheets). A PNG
// keeps its transparency; in a photo or scan (JPEG, or a PNG without
// transparency) the light paper becomes transparent. The ink is trimmed and
// scaled to fit SignatureWidth × SignatureHeight, and stored as PNG.
const (
	MaxSignatureUpload = 4 << 20
	SignatureWidth     = 600
	SignatureHeight    = 200
	maxSignatureStored = 256 << 10
	paperLevel         = 0xd800 // brighter than this (of 0xffff) is paper
)

// ProcessSignature checks and prepares an uploaded signature. It returns the
// PNG to store and a short version.
func ProcessSignature(contentType string, data []byte) ([]byte, string, error) {
	if len(data) < 1 || len(data) > MaxSignatureUpload {
		return nil, "", ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format == "jpeg" && contentType != "image/jpeg") || (format == "png" && contentType != "image/png") || (format != "jpeg" && format != "png") {
		return nil, "", ErrInvalid
	}
	if config.Width < 40 || config.Height < 20 || config.Width > 6000 || config.Height > 6000 {
		return nil, "", ErrInvalid
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", ErrInvalid
	}
	ink := knockOut(src, !hasTransparency(src))
	box, ok := inkBounds(ink)
	if !ok {
		return nil, "", ErrInvalid // nothing but paper
	}
	out := fit(ink, box, SignatureWidth, SignatureHeight)
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil || buf.Len() > maxSignatureStored {
		return nil, "", ErrInvalid
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:6]), nil
}

func hasTransparency(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xf000 {
				return true
			}
		}
	}
	return false
}

// knockOut copies img as non-premultiplied RGBA; with paper true, light
// pixels become transparent and the rest keep their colour, fading out
// towards the paper level so strokes keep smooth edges.
func knockOut(img image.Image, paper bool) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if paper {
				r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				lum := (299*r + 587*g + 114*bl) / 1000
				switch {
				case lum >= paperLevel:
					c.A = 0
				case lum > paperLevel-0x3000:
					c.A = uint8(uint32(c.A) * (paperLevel - lum) / 0x3000)
				}
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// inkBounds is the smallest rectangle around the visible pixels.
func inkBounds(img *image.NRGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, -1, -1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 24 {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// fit scales the box of src down (never up) to fit w × h, averaging the
// source pixels under each output pixel (alpha-weighted colour).
func fit(src *image.NRGBA, box image.Rectangle, w, h int) *image.NRGBA {
	bw, bh := box.Dx(), box.Dy()
	ow, oh := bw, bh
	if ow > w || oh > h {
		if bw*h > bh*w {
			ow, oh = w, max(1, bh*w/bw)
		} else {
			ow, oh = max(1, bw*h/bh), h
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for oy := 0; oy < oh; oy++ {
		sy0, sy1 := box.Min.Y+oy*bh/oh, box.Min.Y+(oy+1)*bh/oh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < ow; ox++ {
			sx0, sx1 := box.Min.X+ox*bw/ow, box.Min.X+(ox+1)*bw/ow
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					c := src.NRGBAAt(x, y)
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			px := color.NRGBA{A: uint8(a / n)}
			if a > 0 {
				px.R, px.G, px.B = uint8(r/a), uint8(g/a), uint8(bl/a)
			}
			dst.SetNRGBA(ox, oy, px)
		}
	}
	return dst
}

// ---- service ----

// SignatureRepository stores signatures (GormRepository).
type SignatureRepository interface {
	SaveSignature(ctx context.Context, actor, id string, content []byte, version string) error
	DeleteSignature(ctx context.Context, actor, id string) error
	Signature(ctx context.Context, id string) ([]byte, string, error)
}

func (s *Service) signatures(ctx context.Context, v Viewer) (SignatureRepository, string, error) {
	repo, ok := s.repo.(SignatureRepository)
	if !ok {
		return nil, "", ErrNotFound
	}
	id, err := s.OwnProfileID(ctx, v)
	return repo, id, err
}

// SaveOwnSignature stores the signed-in member of staff's signature.
func (s *Service) SaveOwnSignature(ctx context.Context, v Viewer, contentType string, data []byte) (string, error) {
	repo, id, err := s.signatures(ctx, v)
	if err != nil {
		return "", err
	}
	content, version, err := ProcessSignature(contentType, data)
	if err != nil {
		return "", err
	}
	return version, repo.SaveSignature(ctx, v.ID, id, content, version)
}

// DeleteOwnSignature removes the signed-in member of staff's signature.
func (s *Service) DeleteOwnSignature(ctx context.Context, v Viewer) error {
	repo, id, err := s.signatures(ctx, v)
	if err != nil {
		return err
	}
	return repo.DeleteSignature(ctx, v.ID, id)
}

// OwnSignature returns the signed-in member of staff's signature and its
// version.
func (s *Service) OwnSignature(ctx context.Context, v Viewer) ([]byte, string, error) {
	repo, id, err := s.signatures(ctx, v)
	if err != nil {
		return nil, "", err
	}
	return repo.Signature(ctx, id)
}

// ---- repository ----

func (r *GormRepository) SaveSignature(ctx context.Context, actor, id string, content []byte, version string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO employee_signatures (employee_id, content, version, updated_by) VALUES (?, ?, ?, ?)
ON CONFLICT (employee_id) DO UPDATE SET content = EXCLUDED.content, version = EXCLUDED.version, updated_by = EXCLUDED.updated_by, updated_at = now()`, id, content, version, actor).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "staff.signature_saved", "employee", id, map[string]any{"bytes": len(content)})
	})
}

func (r *GormRepository) DeleteSignature(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM employee_signatures WHERE employee_id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "staff.signature_deleted", "employee", id, nil)
	})
}

func (r *GormRepository) Signature(ctx context.Context, id string) ([]byte, string, error) {
	var rows []struct {
		Content []byte
		Version string
	}
	if err := r.db.WithContext(ctx).Raw("SELECT content, version FROM employee_signatures WHERE employee_id = ?", id).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", ErrNotFound
	}
	return rows[0].Content, rows[0].Version, nil
}
