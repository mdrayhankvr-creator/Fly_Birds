package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDeployDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "deploy with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"compose.prod.yaml", "school.env.example", "Caddyfile"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCommand(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"remove"}, {"create"}, {"create", "--code", "alpha"}, {"create", "--code", "alpha", "--domain", "a.flybirds.com", "extra"}} {
		if code, _, _ := runCommand(args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestCreateAndList(t *testing.T) {
	dir := testDeployDir(t)
	code, _, stderr := runCommand("create", "--code", "Bad_Code", "--domain", "a.flybirds.com", "--deploy-dir", dir)
	if code != 1 || !strings.Contains(stderr, "school code") {
		t.Fatalf("invalid code: exit %d, %s", code, stderr)
	}
	code, stdout, _ := runCommand("list", "--deploy-dir", dir)
	if code != 0 || !strings.Contains(stdout, "No schools") {
		t.Fatalf("empty list: exit %d, %s", code, stdout)
	}
	code, stdout, stderr = runCommand("create", "--code", "alpha", "--domain", "alpha.localhost", "--dry-run", "--skip-docker-check", "--deploy-dir", dir)
	if code != 0 || !strings.Contains(stdout, "Dry run") {
		t.Fatalf("dry run: exit %d, %s %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "alpha.env")); !os.IsNotExist(err) {
		t.Fatal("the dry run wrote alpha.env")
	}
	code, _, stderr = runCommand("create", "--code", "alpha", "--domain", "alpha.localhost", "--skip-docker-check", "--web-port", "18081", "--deploy-dir", dir)
	if code != 0 {
		t.Fatalf("create: exit %d, %s", code, stderr)
	}
	code, stdout, _ = runCommand("list", "--deploy-dir", dir)
	if code != 0 || !strings.Contains(stdout, "alpha") || !strings.Contains(stdout, "18081") {
		t.Fatalf("list: exit %d, %s", code, stdout)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "alpha.env"))
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok && (key == "POSTGRES_PASSWORD" || key == "JWT_SIGNING_KEY_B64") && strings.Contains(stdout, value) {
			t.Errorf("list printed %s", key)
		}
	}
}
