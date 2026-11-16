package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectsApplyAndPasswordManifest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--apply"}, &stdout, &stderr); code != 2 {
		t.Fatalf("apply flag accepted: %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatal("apply printed a report")
	}
	path := filepath.Join(t.TempDir(), "identifiers.json")
	if err := os.WriteFile(path, []byte(`{"account_emails":["new@example.test"],"password":"never print this"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatal("password field accepted")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--manifest", path}, &stdout, &stderr); code != 2 {
		t.Fatalf("unsafe manifest accepted: %d", code)
	}
	if strings.Contains(stderr.String(), "never print this") || strings.Contains(stdout.String(), "never print this") {
		t.Fatal("password leaked into command output")
	}
	if err := os.WriteFile(path, []byte(`null`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatal("empty manifest accepted as a completed conflict check")
	}
}
