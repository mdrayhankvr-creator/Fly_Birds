package admin

import (
	"testing"
	"time"
)

func TestSafeCellNeutralisesFormulas(t *testing.T) {
	for in, want := range map[string]string{
		"=SUM(A1)": "'=SUM(A1)", "+880": "'+880", "-2": "'-2", "@cmd": "'@cmd", "\tx": "'\tx", "\rx": "'\rx",
		"": "", "Rahim": "Rahim", "রহিম": "রহিম", "a=b": "a=b",
	} {
		if got := SafeCell(in); got != want {
			t.Errorf("SafeCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExportRange(t *testing.T) {
	for _, c := range []struct {
		from, to string
		ok       bool
	}{
		{"2026-01-01", "2026-01-01", true},
		{"2025-01-01", "2026-02-04", true},
		{"2025-01-01", "2026-02-06", false},
		{"2026-02-02", "2026-02-01", false},
		{"2026-02-30", "2026-03-01", false},
		{"", "", false},
	} {
		if _, _, ok := ExportRange(c.from, c.to); ok != c.ok {
			t.Errorf("ExportRange(%s, %s) = %v", c.from, c.to, ok)
		}
	}
}

func TestFileNameIgnoresInvalidRanges(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	if got := FileName("ledger", "2026-01-01", "2026-01-31", now); got != "flybirds-ledger-2026-01-01-to-2026-01-31.csv" {
		t.Fatal(got)
	}
	if got := FileName("users", `x"; evil`, "y", now); got != "flybirds-users-20261018.csv" {
		t.Fatal(got)
	}
}
