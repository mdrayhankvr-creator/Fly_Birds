package staff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	_ "image/png" // PNG uploads are accepted and stored as JPEG
)

const (
	MaxPhotoUpload = 6 << 20 // bytes accepted from the browser
	PhotoSize      = 480     // stored square edge, in pixels
	maxPhotoStored = 512 << 10
)

// ProcessPhoto checks an uploaded JPEG or PNG, crops it to a centred square,
// scales it down to PhotoSize and re-encodes it as JPEG, which also drops
// embedded metadata such as the camera location. It returns the stored bytes
// and a short version used to refresh cached copies.
func ProcessPhoto(contentType string, data []byte) ([]byte, string, error) {
	if len(data) < 1 || len(data) > MaxPhotoUpload {
		return nil, "", ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format == "jpeg" && contentType != "image/jpeg") || (format == "png" && contentType != "image/png") || (format != "jpeg" && format != "png") {
		return nil, "", ErrInvalid
	}
	if config.Width < 64 || config.Height < 64 || config.Width > 8000 || config.Height > 8000 {
		return nil, "", ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", ErrInvalid
	}
	square := squareDown(decoded, PhotoSize)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, square, &jpeg.Options{Quality: 82}); err != nil || out.Len() > maxPhotoStored {
		return nil, "", ErrInvalid
	}
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(sum[:6]), nil
}

// squareDown crops the centred square of src and scales it to size×size by
// averaging the source pixels under each output pixel (smaller images are
// scaled up the same way). Transparent pixels are flattened onto white.
func squareDown(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for oy := 0; oy < size; oy++ {
		sy0, sy1 := y0+oy*side/size, y0+(oy+1)*side/size
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < size; ox++ {
			sx0, sx1 := x0+ox*side/size, x0+(ox+1)*side/size
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					cr, cg, cb, ca := src.At(x, y).RGBA()
					// Composite over white (colours are premultiplied by alpha).
					white := 0xffff - uint64(ca)
					r += uint64(cr) + white
					g += uint64(cg) + white
					bl += uint64(cb) + white
					n++
				}
			}
			i := dst.PixOffset(ox, oy)
			dst.Pix[i+0] = uint8(r / n >> 8)
			dst.Pix[i+1] = uint8(g / n >> 8)
			dst.Pix[i+2] = uint8(bl / n >> 8)
			dst.Pix[i+3] = 0xff
		}
	}
	return dst
}
