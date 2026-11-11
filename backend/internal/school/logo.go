package school

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"regexp"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// The uploaded school logo (migration 1761). After an upload logo_url is
// UploadedLogoPath plus "?v=<version>", so pages show it like any logo link
// and a new upload is never served from an old cache.

const (
	UploadedLogoPath = "/api/school/logo"
	MaxLogoUpload    = 6 << 20 // bytes accepted from the browser
	LogoEdge         = 512     // longest stored side, in pixels
	maxLogoStored    = 512 << 10
)

var uploadedLogoURL = regexp.MustCompile(`^/api/school/logo\?v=[0-9a-f]{12}$`)

// IsUploadedLogo reports whether a logo_url points at the uploaded logo.
func IsUploadedLogo(raw string) bool { return uploadedLogoURL.MatchString(raw) }

// ProcessLogo checks an uploaded JPEG or PNG, scales it down to at most
// LogoEdge on its longer side and re-encodes it: PNG when it has transparent
// pixels (logos often do), JPEG otherwise. Re-encoding drops any metadata.
func ProcessLogo(contentType string, data []byte) ([]byte, string, string, error) {
	if len(data) < 1 || len(data) > MaxLogoUpload {
		return nil, "", "", ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format == "jpeg" && contentType != "image/jpeg") || (format == "png" && contentType != "image/png") || (format != "jpeg" && format != "png") {
		return nil, "", "", ErrInvalid
	}
	if config.Width < 16 || config.Height < 16 || config.Width > 8000 || config.Height > 8000 {
		return nil, "", "", ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", "", ErrInvalid
	}
	scaled := scaleKeepingAlpha(decoded, LogoEdge)
	var out bytes.Buffer
	kind := "image/jpeg"
	if transparent(scaled) {
		kind = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, scaled)
	} else {
		err = jpeg.Encode(&out, onWhite(scaled), &jpeg.Options{Quality: 88})
	}
	if err != nil || out.Len() > maxLogoStored {
		return nil, "", "", ErrInvalid
	}
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), kind, hex.EncodeToString(sum[:6]), nil
}

// scaleKeepingAlpha returns src scaled so its longer side is at most edge, averaging
// the source pixels under each output pixel (premultiplied, so transparent
// edges stay clean). Smaller images are copied unchanged.
func scaleKeepingAlpha(src image.Image, edge int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > edge || h > edge {
		if w >= h {
			w, h = edge, max(1, b.Dy()*edge/b.Dx())
		} else {
			w, h = max(1, b.Dx()*edge/b.Dy()), edge
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for oy := 0; oy < h; oy++ {
		sy0, sy1 := b.Min.Y+oy*b.Dy()/h, b.Min.Y+(oy+1)*b.Dy()/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < w; ox++ {
			sx0, sx1 := b.Min.X+ox*b.Dx()/w, b.Min.X+(ox+1)*b.Dx()/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					cr, cg, cb, ca := src.At(x, y).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			i := dst.PixOffset(ox, oy)
			dst.Pix[i+0], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}

func transparent(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 0xf0 {
			return true
		}
	}
	return false
}

// onWhite flattens any faint transparency onto white for JPEG.
func onWhite(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Rect)
	for i := 0; i < len(img.Pix); i += 4 {
		white := 0xff - img.Pix[i+3]
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = img.Pix[i]+white, img.Pix[i+1]+white, img.Pix[i+2]+white, 0xff
	}
	return out
}

// LogoStore keeps the uploaded logo.
type LogoStore struct{ db *gorm.DB }

func NewLogoStore(db *gorm.DB) *LogoStore { return &LogoStore{db: db} }

// Logo is the stored logo.
type Logo struct {
	Content     []byte
	ContentType string
	Version     string
}

// Get returns the uploaded logo, or ErrNotFound.
func (l *LogoStore) Get(ctx context.Context) (Logo, error) {
	return readLogo(l.db.WithContext(ctx))
}

func readLogo(db *gorm.DB) (Logo, error) {
	var out Logo
	err := db.Raw("SELECT content, content_type, version FROM school_logo WHERE id = 1").Row().Scan(&out.Content, &out.ContentType, &out.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Logo{}, ErrNotFound
	}
	return out, err
}

// ReadUploadedLogo reads the uploaded logo with any database handle; used by
// documents, which print it.
func ReadUploadedLogo(ctx context.Context, db *gorm.DB) ([]byte, string, error) {
	logo, err := readLogo(db.WithContext(ctx))
	return logo.Content, logo.ContentType, err
}

// Save processes and stores a new logo and points logo_url at it.
func (l *LogoStore) Save(ctx context.Context, actor, contentType string, data []byte) (string, error) {
	content, kind, version, err := ProcessLogo(contentType, data)
	if err != nil {
		return "", err
	}
	logoURL := UploadedLogoPath + "?v=" + version
	err = l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "school_settings:update"); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO school_logo (id, content_type, content, version, updated_by) VALUES (1, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET content_type = EXCLUDED.content_type, content = EXCLUDED.content, version = EXCLUDED.version, updated_by = EXCLUDED.updated_by, updated_at = now()`, kind, content, version, actor).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE school_settings SET logo_url = ?, updated_at = now() WHERE id = ?", logoURL, SingleSchoolID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.logo_uploaded", "school", "1", map[string]any{"content_type": kind, "bytes": len(content), "version": version})
	})
	return logoURL, err
}

// Delete removes the uploaded logo; logo_url is cleared when it pointed at it.
func (l *LogoStore) Delete(ctx context.Context, actor string) error {
	return l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "school_settings:update"); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM school_logo WHERE id = 1").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE school_settings SET logo_url = '', updated_at = now() WHERE id = ? AND logo_url LIKE '/api/school/logo?v=%'", SingleSchoolID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.logo_removed", "school", "1", nil)
	})
}
