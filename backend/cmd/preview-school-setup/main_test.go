package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/schoolsetup"
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

func TestManifestOutputCannotOverwriteAndGenerationNeedsSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	first := schoolsetup.Manifest{SchemaVersion: schoolsetup.ManifestSchemaVersion, Session: "2026", SnapshotSHA256: "example"}
	if err := writeManifestExclusive(path, first); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManifestExclusive(path, schoolsetup.Manifest{Session: "different"}); err == nil {
		t.Fatal("existing manifest was overwritten")
	}
	again, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("existing manifest changed")
	}
	if _, err := readManifest(path); err != nil {
		t.Fatalf("complete zero-candidate manifest rejected: %v", err)
	}
	for _, args := range [][]string{
		{"--generate-manifest", path},
		{"--generate-manifest", "-", "--session", "2026"},
		{"--generate-manifest", path, "--session", "2026", "--manifest", path},
		{"--generate-manifest", path, "--session", "2026", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		if got := run(args, &stdout, &stderr); got != 2 {
			t.Fatalf("unsafe flags accepted: %v (%d)", args, got)
		}
	}
}
