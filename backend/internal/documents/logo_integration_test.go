package documents

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	schoolcfg "github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresUploadedLogoOnDocuments: an uploaded logo is stored resized,
// logo_url points at it, documents print it without downloading anything,
// and removing it clears logo_url so documents fall back to the initials.
func TestPostgresUploadedLogoOnDocuments(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	var admin string
	if err := tx.Raw("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'Logo admin') RETURNING id", fmt.Sprintf("logo-%d@example.test", time.Now().UnixNano())).Row().Scan(&admin); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1200; x++ {
			a := uint8(255)
			if (x-600)*(x-600)+(y-600)*(y-600) > 500*500 {
				a = 0 // a round logo on a transparent background
			}
			img.Set(x, y, color.NRGBA{10, 80, 160, a})
		}
	}
	var upload bytes.Buffer
	if err := png.Encode(&upload, img); err != nil {
		t.Fatal(err)
	}

	store := schoolcfg.NewLogoStore(tx)
	// Downloads must not be used for an uploaded logo.
	svc := &Service{db: tx, Logo: func(context.Context, string) ([]byte, error) { t.Fatal("downloaded an uploaded logo"); return nil, nil }}

	url, err := store.Save(ctx, admin, "image/png", upload.Bytes())
	if err != nil || !schoolcfg.IsUploadedLogo(url) {
		t.Fatalf("save: %q %v", url, err)
	}
	var setting string
	tx.Raw("SELECT logo_url FROM school_settings WHERE id = 1").Scan(&setting)
	if setting != url {
		t.Fatalf("logo_url %q, want %q", setting, url)
	}
	stored, err := store.Get(ctx)
	if err != nil || stored.ContentType != "image/png" || len(stored.Content) >= upload.Len() {
		t.Fatalf("stored %s %d bytes (upload %d) %v", stored.ContentType, len(stored.Content), upload.Len(), err)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(stored.Content)); err != nil || cfg.Width != schoolcfg.LogoEdge {
		t.Fatalf("stored size %+v %v", cfg, err)
	}
	b, err := svc.branding(ctx, lang("en"))
	if err != nil || b.school.Logo != "logo.png" || !bytes.Equal(b.logo, stored.Content) {
		t.Fatalf("branding logo %q %d bytes %v", b.school.Logo, len(b.logo), err)
	}
	key := b.key

	// Removing it clears logo_url: documents use the initials and a new key.
	if err := store.Delete(ctx, admin); err != nil {
		t.Fatal(err)
	}
	tx.Raw("SELECT logo_url FROM school_settings WHERE id = 1").Scan(&setting)
	if setting != "" {
		t.Fatalf("logo_url after delete %q", setting)
	}
	if _, err := store.Get(ctx); err != schoolcfg.ErrNotFound {
		t.Fatalf("get after delete: %v", err)
	}
	b, err = svc.branding(ctx, lang("en"))
	if err != nil || b.school.Logo != "" || b.key == key {
		t.Fatalf("branding after delete %q %v", b.school.Logo, err)
	}
	// A web link set later is not touched by removing the upload.
	tx.Exec("UPDATE school_settings SET logo_url = 'https://cdn.example.com/logo.png' WHERE id = 1")
	if err := store.Delete(ctx, admin); err != nil {
		t.Fatal(err)
	}
	tx.Raw("SELECT logo_url FROM school_settings WHERE id = 1").Scan(&setting)
	if setting != "https://cdn.example.com/logo.png" {
		t.Fatalf("web link cleared: %q", setting)
	}
}
