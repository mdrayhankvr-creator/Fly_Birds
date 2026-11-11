package demo

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// The drawn gallery photos and portrait must pass the same checks as an
// administrator's upload. DEMO_SCENES_DIR saves them for a look.
func TestDemoPicturesAreValidUploads(t *testing.T) {
	pictures := map[string]*image.RGBA{"portrait": drawPortrait()}
	for _, scene := range galleryScenes {
		if scene.en == "" || scene.bn == "" || len(scene.en) > 140 {
			t.Errorf("%s: caption %q / %q", scene.ref, scene.en, scene.bn)
		}
		pictures[scene.ref] = scene.draw()
	}
	for name, img := range pictures {
		data, err := pngBytes(img)
		if err != nil {
			t.Fatal(err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" || cfg.Width < 200 || cfg.Height < 200 || len(data) > school.MaxGalleryUpload {
			t.Errorf("%s: %s %dx%d, %d bytes, %v", name, format, cfg.Width, cfg.Height, len(data), err)
		}
		if dir := os.Getenv("DEMO_SCENES_DIR"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, name+".png"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Drawing is deterministic, so a repeat run stores the same photo.
	a, _ := pngBytes(drawLibrary())
	b, _ := pngBytes(drawLibrary())
	if !bytes.Equal(a, b) {
		t.Error("the library drawing changes between runs")
	}
}
