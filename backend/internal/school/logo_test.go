package school

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func logoImage(t *testing.T, w, h int, transparent bool, kind string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if transparent && (x < w/4 || y < h/4) {
				a = 0
			}
			img.Set(x, y, color.NRGBA{20, 90, uint8(x), a})
		}
	}
	var b bytes.Buffer
	var err error
	if kind == "image/png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestProcessLogo(t *testing.T) {
	// A large transparent PNG keeps its transparency and is scaled down,
	// keeping its shape.
	out, kind, version, err := ProcessLogo("image/png", logoImage(t, 1600, 800, true, "image/png"))
	if err != nil || kind != "image/png" || len(version) != 12 {
		t.Fatalf("png: %v %s %q", err, kind, version)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil || format != "png" || img.Bounds().Dx() != LogoEdge || img.Bounds().Dy() != LogoEdge/2 {
		t.Fatalf("png decoded %v %s %v", err, format, img.Bounds())
	}
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Fatalf("corner alpha %d, want transparent", a)
	}
	// An opaque PNG becomes a JPEG; a small one keeps its size.
	out, kind, _, err = ProcessLogo("image/png", logoImage(t, 200, 300, false, "image/png"))
	if err != nil || kind != "image/jpeg" {
		t.Fatalf("opaque png: %v %s", err, kind)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "jpeg" || cfg.Width != 200 || cfg.Height != 300 {
		t.Fatalf("opaque decoded %v %s %+v", err, format, cfg)
	}
	// The same input gives the same version; another gives another.
	_, _, v1, _ := ProcessLogo("image/jpeg", logoImage(t, 300, 300, false, "image/jpeg"))
	_, _, v2, _ := ProcessLogo("image/jpeg", logoImage(t, 300, 300, false, "image/jpeg"))
	_, _, v3, _ := ProcessLogo("image/jpeg", logoImage(t, 301, 300, false, "image/jpeg"))
	if v1 != v2 || v1 == v3 {
		t.Fatalf("versions %s %s %s", v1, v2, v3)
	}
	// Refused: not an image, a type that does not match, too small.
	for name, c := range map[string]struct {
		kind string
		data []byte
	}{
		"garbage":  {"image/png", []byte("hello")},
		"mismatch": {"image/jpeg", logoImage(t, 100, 100, false, "image/png")},
		"tiny":     {"image/png", logoImage(t, 8, 8, false, "image/png")},
	} {
		if _, _, _, err := ProcessLogo(c.kind, c.data); err != ErrInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
	// logo_url values the settings accept for an uploaded logo.
	if !IsUploadedLogo("/api/school/logo?v=0123456789ab") || IsUploadedLogo("/api/school/logo") || IsUploadedLogo("/api/school/logo?v=zz") || !validLogoURL("/api/school/logo?v=0123456789ab") || validLogoURL("/api/school/other?v=0123456789ab") {
		t.Fatal("uploaded logo url checks")
	}
}
