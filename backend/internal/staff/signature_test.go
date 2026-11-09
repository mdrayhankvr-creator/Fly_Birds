package staff

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestProcessSignature: a PNG keeps its transparency; a scan on white paper
// loses the paper; the ink is trimmed and fitted to the stored size; blank
// and broken uploads are refused.
func TestProcessSignature(t *testing.T) {
	decode := func(t *testing.T, b []byte) image.Image {
		t.Helper()
		img, format, err := image.Decode(bytes.NewReader(b))
		if err != nil || format != "png" {
			t.Fatalf("stored %s: %v", format, err)
		}
		return img
	}
	// A wide stroke on transparent paper, with empty margins.
	clear := image.NewNRGBA(image.Rect(0, 0, 1600, 400))
	for x := 200; x < 1400; x++ {
		for y := 180; y < 220; y++ {
			clear.SetNRGBA(x, y, color.NRGBA{10, 20, 120, 255})
		}
	}
	out, version, err := ProcessSignature("image/png", encode(t, clear, "png"))
	if err != nil || len(version) != 12 {
		t.Fatalf("png: %v %q", err, version)
	}
	got := decode(t, out)
	// Trimmed to the 1200 × 40 stroke and scaled to fit 600 wide.
	if got.Bounds().Dx() != SignatureWidth || got.Bounds().Dy() != 20 {
		t.Fatalf("size %v", got.Bounds())
	}
	if _, _, _, a := got.At(300, 10).RGBA(); a < 0xf000 {
		t.Fatal("the ink lost its colour")
	}

	// A scan: dark ink on white paper (no transparency). The paper goes.
	scan := image.NewRGBA(image.Rect(0, 0, 300, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 300; x++ {
			c := color.RGBA{250, 250, 245, 255}
			// Two strokes, so the trimmed image keeps some paper.
			if (y >= 50 && y < 60 && x >= 30 && x < 150) || (y >= 70 && y < 80 && x >= 150 && x < 270) {
				c = color.RGBA{30, 30, 40, 255}
			}
			scan.Set(x, y, c)
		}
	}
	for _, kind := range []string{"png", "jpeg"} {
		out, _, err := ProcessSignature("image/"+kind, encode(t, scan, map[string]string{"png": "png", "jpeg": "jpg"}[kind]))
		if err != nil {
			t.Fatalf("%s scan: %v", kind, err)
		}
		got := decode(t, out)
		b := got.Bounds()
		if b.Dx() > 250 || b.Dy() > 35 {
			t.Fatalf("%s scan not trimmed to the ink: %v", kind, b)
		}
		if _, _, _, a := got.At(b.Dx()/4, 3).RGBA(); a < 0xc000 {
			t.Fatalf("%s scan: the ink became transparent", kind)
		}
		// The paper below the first stroke is transparent.
		if _, _, _, a := got.At(1, b.Dy()-2).RGBA(); a > 0x2000 {
			t.Fatalf("%s scan: paper kept at the corner", kind)
		}
	}

	blank := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for i := range blank.Pix {
		blank.Pix[i] = 255
	}
	var white bytes.Buffer
	_ = png.Encode(&white, blank)
	for name, c := range map[string]struct {
		kind string
		data []byte
	}{
		"blank paper": {"image/png", white.Bytes()},
		"wrong type":  {"image/jpeg", encode(t, clear, "png")},
		"not image":   {"image/png", []byte("not an image")},
		"too small":   {"image/png", encode(t, image.NewNRGBA(image.Rect(0, 0, 30, 10)), "png")},
		"unsupported": {"image/gif", encode(t, clear, "png")},
	} {
		if _, _, err := ProcessSignature(c.kind, c.data); err != ErrInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
}
