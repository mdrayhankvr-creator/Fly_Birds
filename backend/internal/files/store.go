// Package files stores uploaded attachments outside PostgreSQL. Files are kept
// under random server-generated keys, so user-supplied names never reach the
// filesystem, and they are only served through authorized API endpoints.
package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var (
	ErrInvalid  = errors.New("invalid file")
	ErrNotFound = errors.New("file not found")
	ErrTooLarge = errors.New("file too large")
	ErrType     = errors.New("unsupported file type")
)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Store is the storage boundary; LocalStore can be replaced by object storage.
type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type LocalStore struct{ root string }

func NewLocalStore(root string) (*LocalStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func NewKey() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// path accepts only generated keys, so traversal sequences cannot be formed.
func (s *LocalStore) path(key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", ErrInvalid
	}
	return filepath.Join(s.root, key[:2], key), nil
}

// Put writes to a temporary file and renames it, so readers never observe a
// partially written attachment.
func (s *LocalStore) Put(_ context.Context, key string, data []byte) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

func (s *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	target, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
