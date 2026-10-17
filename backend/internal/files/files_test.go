package files

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{200, 10, 10, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const minimalPDF = "%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n"

func TestValidateIdentifiesFilesByContent(t *testing.T) {
	pdf, err := Validate("Lesson plan.PDF", []byte(minimalPDF))
	if err != nil || pdf.ContentType != "application/pdf" || pdf.Name != "Lesson plan.pdf" || len(pdf.SHA256) != 64 {
		t.Fatalf("pdf %+v %v", pdf, err)
	}
	pic, err := Validate("photo.jpg", pngBytes(t, 20, 10))
	if err != nil || pic.ContentType != "image/png" || pic.Name != "photo.png" {
		t.Fatalf("png named jpg %+v %v", pic.ContentType, err)
	}
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, image.NewGray(image.Rect(0, 0, 8, 8)), nil)
	if j, err := Validate("x.jpeg", jb.Bytes()); err != nil || j.ContentType != "image/jpeg" {
		t.Fatalf("jpeg %v", err)
	}
	for name, data := range map[string][]byte{
		"empty":          {},
		"html as pdf":    []byte("<html><script>alert(1)</script></html>"),
		"exe":            []byte("MZ\x90\x00binary"),
		"truncated pdf":  []byte("%PDF-1.4\n1 0 obj"),
		"javascript pdf": []byte("%PDF-1.4\n<< /OpenAction << /S /JavaScript /JS (app.alert(1)) >> >>\n%%EOF"),
		"launch pdf":     []byte("%PDF-1.4\n<< /S /Launch /F (cmd.exe) >>\n%%EOF"),
		"fake png":       append([]byte("\x89PNG\r\n\x1a\n"), []byte("not really")...),
		"svg":            []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`),
	} {
		if _, err := Validate(name, data); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := Validate("big.pdf", append([]byte(minimalPDF), make([]byte, MaxPDFBytes)...)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized pdf: %v", err)
	}
	if _, err := Validate("huge.png", pngBytes(t, maxDimension+1, 1)); err == nil {
		t.Fatal("oversized image dimensions accepted")
	}
}

func TestSafeNameStripsPathsAndControls(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":          "passwd.pdf",
		`..\..\windows\win.ini`:     "win.pdf",
		"report\"; filename=x.html": "report filename=x.pdf",
		"\x00\x1fনোটিশ.pdf":         "নোটিশ.pdf",
		"":                          "attachment.pdf",
		"..":                        "attachment.pdf",
	} {
		if got := SafeName(in, "application/pdf"); got != want {
			t.Fatalf("SafeName(%q) = %q want %q", in, got, want)
		}
	}
	if got := SafeName(strings.Repeat("ক", 100), "image/png"); len(got) > 120 || !strings.HasSuffix(got, ".png") {
		t.Fatalf("long name %d %q", len(got), got)
	}
}

func TestLocalStoreRejectsForeignKeys(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, key := range []string{"../escape", "..", "ABCDEF0123456789abcdef0123456789", "abc", "0123456789abcdef0123456789abcdef/../x"} {
		if err := store.Put(ctx, key, []byte("x")); !errors.Is(err, ErrInvalid) {
			t.Fatalf("key %q accepted: %v", key, err)
		}
	}
	key, _ := NewKey()
	if err := store.Put(ctx, key, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	r, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r)
	r.Close()
	if string(body) != "hello" {
		t.Fatalf("read %q", body)
	}
	if info, err := os.Stat(filepath.Join(dir, key[:2], key)); err != nil || info.IsDir() {
		t.Fatalf("stored path %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted file opened: %v", err)
	}
}
