package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxPDFBytes   = 10 << 20
	MaxImageBytes = 5 << 20
	maxDimension  = 6000
)

// Clean is an upload that passed validation and is ready to store.
type Clean struct {
	Name        string
	ContentType string
	Data        []byte
	SHA256      string
}

// pdfActive lists PDF features that run code or open other content. Such files
// are rejected; this is a best-effort check, not a full PDF parser.
var pdfActive = [][]byte{[]byte("/JavaScript"), []byte("/JS "), []byte("/JS("), []byte("/Launch"), []byte("/EmbeddedFile"), []byte("/RichMedia")}

// Validate identifies the file from its bytes, never from the client's name or
// declared type. PDFs must be well-formed at both ends and free of active
// content; JPEG and PNG images are decoded and re-encoded to strip metadata.
func Validate(name string, data []byte) (Clean, error) {
	if len(data) == 0 {
		return Clean{}, ErrInvalid
	}
	var out Clean
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		if len(data) > MaxPDFBytes {
			return Clean{}, ErrTooLarge
		}
		tail := data[max(0, len(data)-2048):]
		if !bytes.Contains(tail, []byte("%%EOF")) {
			return Clean{}, ErrType
		}
		for _, marker := range pdfActive {
			if bytes.Contains(data, marker) {
				return Clean{}, ErrType
			}
		}
		out = Clean{ContentType: "application/pdf", Data: data}
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}), bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		if len(data) > MaxImageBytes {
			return Clean{}, ErrTooLarge
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > maxDimension || config.Height > maxDimension {
			return Clean{}, ErrType
		}
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return Clean{}, ErrType
		}
		var clean bytes.Buffer
		switch format {
		case "jpeg":
			err = jpeg.Encode(&clean, decoded, &jpeg.Options{Quality: 88})
			out.ContentType = "image/jpeg"
		case "png":
			err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&clean, decoded)
			out.ContentType = "image/png"
		default:
			return Clean{}, ErrType
		}
		if err != nil || clean.Len() > MaxImageBytes {
			return Clean{}, ErrTooLarge
		}
		out.Data = clean.Bytes()
	default:
		return Clean{}, ErrType
	}
	out.Name = SafeName(name, out.ContentType)
	sum := sha256.Sum256(out.Data)
	out.SHA256 = hex.EncodeToString(sum[:])
	return out, nil
}

var extensions = map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png"}

// SafeName keeps a readable base name for downloads: no directories, control
// characters or quotes, at most 120 bytes, and the extension of the real type.
func SafeName(name, contentType string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base("/" + name)
	if !utf8.ValidString(name) {
		name = ""
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`"/\<>:|?*;`, r) {
			return -1
		}
		return r
	}, name)
	base := strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
	for len(base) > 110 {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	if base == "" || base == "." {
		base = "attachment"
	}
	return base + extensions[contentType]
}
