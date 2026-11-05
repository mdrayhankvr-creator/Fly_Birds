package documents

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Documents are rendered by Typst, a single binary in the backend image
// (E7 spike: correct Bangla shaping, about 0.5 s and 13 MB per marksheet).
// Each render runs in its own temporary folder with only the bundled fonts,
// no packages and no network access of its own; school data never leaves
// the server.

//go:embed templates/*.typ
var templates embed.FS

// ErrUnavailable means the renderer is not installed or not configured.
var ErrUnavailable = errors.New("document renderer unavailable")

// Render is one document to render: a template, its data and its images.
type Render struct {
	Template string            // file in templates/, e.g. marksheet.typ
	Data     any               // written as data.json
	Assets   map[string][]byte // extra files, by name (logo.png, qr-0.png …)
	// Created is the date written into the PDF, so the same inputs give the
	// same file.
	Created time.Time
}

// Renderer turns a Render into a PDF.
type Renderer interface {
	Render(ctx context.Context, r Render) ([]byte, error)
}

// Typst renders with the typst binary.
type Typst struct {
	Bin     string
	FontDir string
	Timeout time.Duration
	slots   chan struct{}
}

// NewTypst finds the binary and the fonts; it returns ErrUnavailable when
// either is missing. At most `parallel` renders run at once.
func NewTypst(bin, fontDir string, timeout time.Duration, parallel int) (*Typst, error) {
	if bin == "" {
		bin = "typst"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, ErrUnavailable
	}
	if st, err := os.Stat(fontDir); err != nil || !st.IsDir() {
		return nil, ErrUnavailable
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if parallel < 1 {
		parallel = 1
	}
	return &Typst{Bin: path, FontDir: fontDir, Timeout: timeout, slots: make(chan struct{}, parallel)}, nil
}

var safeAsset = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

func (t *Typst) Render(ctx context.Context, r Render) ([]byte, error) {
	select {
	case t.slots <- struct{}{}:
		defer func() { <-t.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	dir, err := os.MkdirTemp("", "flybirds-doc-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		raw, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o600); err != nil {
			return nil, err
		}
	}
	data, err := json.Marshal(r.Data)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), data, 0o600); err != nil {
		return nil, err
	}
	for name, raw := range r.Assets {
		if !safeAsset.MatchString(name) {
			return nil, fmt.Errorf("asset name %q", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			return nil, err
		}
	}
	empty := filepath.Join(dir, ".packages")
	if err := os.Mkdir(empty, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	created := r.Created
	if created.IsZero() {
		created = time.Now()
	}
	cmd := exec.CommandContext(ctx, t.Bin, "compile", "--root", dir, "--ignore-system-fonts", "--font-path", t.FontDir,
		"--package-path", empty, "--package-cache-path", empty, "--creation-timestamp", strconv.FormatInt(created.Unix(), 10),
		"--input", "data=data.json", r.Template, "out.pdf")
	cmd.Dir = dir
	// No proxy or certificate settings reach the renderer.
	cmd.Env = []string{"HOME=" + dir, "PATH=/usr/local/bin:/usr/bin:/bin", "TYPST_FONT_PATHS=" + t.FontDir}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("render timed out after %s", t.Timeout)
		}
		msg := stderr.String()
		if len(msg) > 600 {
			msg = msg[:600]
		}
		return nil, fmt.Errorf("render failed: %v: %s", err, msg)
	}
	return os.ReadFile(filepath.Join(dir, "out.pdf"))
}

var pageCount = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)`)

// Pages reads a PDF's page count (Typst writes its page tree uncompressed).
func Pages(pdf []byte) int {
	m := pageCount.FindSubmatch(pdf)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}
