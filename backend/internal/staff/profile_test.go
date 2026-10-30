package staff

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestMaskNID(t *testing.T) {
	for in, want := range map[string]string{"": "", "1234567890": "•••••••890", "19901234567890123": "••••••••••••••123"} {
		if got := MaskNID(in); got != want {
			t.Errorf("MaskNID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShared(t *testing.T) {
	both, staffOnly := Audience{Staff: true, Families: true}, Audience{Staff: true}
	for _, c := range []struct {
		a    Audience
		role string
		want bool
	}{{staffOnly, "teacher", true}, {staffOnly, "accountant", true}, {staffOnly, "student", false}, {staffOnly, "guardian", false}, {both, "guardian", true}, {Audience{}, "teacher", false}, {both, "unknown", false}} {
		if got := shared(c.a, Viewer{Role: c.role}); got != c.want {
			t.Errorf("shared(%+v, %s) = %v", c.a, c.role, got)
		}
	}
}

func encode(t *testing.T, img image.Image, kind string) []byte {
	t.Helper()
	var b bytes.Buffer
	var err error
	if kind == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestProcessPhoto(t *testing.T) {
	wide := image.NewNRGBA(image.Rect(0, 0, 900, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 900; x++ {
			// Left and right thirds red, the centre square blue.
			c := color.NRGBA{200, 0, 0, 255}
			if x >= 300 && x < 600 {
				c = color.NRGBA{0, 0, 200, 255}
			}
			wide.Set(x, y, c)
		}
	}
	out, version, err := ProcessPhoto("image/png", encode(t, wide, "png"))
	if err != nil || version == "" {
		t.Fatal(err)
	}
	got, format, err := image.Decode(bytes.NewReader(out))
	if err != nil || format != "jpeg" || got.Bounds().Dx() != PhotoSize || got.Bounds().Dy() != PhotoSize {
		t.Fatalf("stored %s %v %v", format, got.Bounds(), err)
	}
	if r, _, b, _ := got.At(PhotoSize/2, PhotoSize/2).RGBA(); b>>8 < 150 || r>>8 > 60 {
		t.Fatal("the photo was not cropped to its centre")
	}
	clear := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	out, _, err = ProcessPhoto("image/png", encode(t, clear, "png"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ = image.Decode(bytes.NewReader(out))
	if r, g, b, _ := got.At(50, 50).RGBA(); r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatal("transparency was not flattened onto white")
	}
	for name, c := range map[string]struct {
		kind string
		data []byte
	}{
		"wrong type":  {"image/jpeg", encode(t, wide, "png")},
		"not image":   {"image/png", []byte("not an image")},
		"too small":   {"image/png", encode(t, image.NewNRGBA(image.Rect(0, 0, 32, 32)), "png")},
		"empty":       {"image/png", nil},
		"unsupported": {"image/gif", encode(t, wide, "png")},
	} {
		if _, _, err := ProcessPhoto(c.kind, c.data); err != ErrInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
}
