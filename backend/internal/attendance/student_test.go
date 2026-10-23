package attendance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type rosterRepo struct {
	StudentRepository
	saved []RosterInput
}

func (r *rosterRepo) SaveRoster(_ context.Context, _ string, in RosterInput) (RosterResult, error) {
	r.saved = append(r.saved, in)
	return RosterResult{Created: len(in.Entries)}, nil
}

func TestSaveRosterValidation(t *testing.T) {
	const (
		actor   = "11111111-1111-1111-1111-111111111111"
		session = "22222222-2222-2222-2222-222222222222"
		section = "33333333-3333-3333-3333-333333333333"
		student = "44444444-4444-4444-4444-444444444444"
	)
	restore := now
	now = func() time.Time { return time.Date(2026, 10, 17, 19, 30, 0, 0, time.UTC) } // 18 Oct in Dhaka
	t.Cleanup(func() { now = restore })
	repo := &rosterRepo{}
	s := NewStudentService(repo)
	in := func(date string, entries ...RosterEntry) RosterInput {
		return RosterInput{SessionID: session, SectionID: section, AttendanceDate: date, Entries: entries}
	}
	bad := "not-a-uuid"
	many := make([]RosterEntry, MaxRosterEntries+1)
	for i := range many {
		many[i] = RosterEntry{StudentID: student, Status: "present"}
	}
	for i, v := range []RosterInput{
		in("2026-10-18"),
		in("2026-10-19", RosterEntry{StudentID: student, Status: "present"}),
		in("2026-02-30", RosterEntry{StudentID: student, Status: "present"}),
		in("2026-10-18", RosterEntry{StudentID: student, Status: "leave"}),
		in("2026-10-18", RosterEntry{StudentID: student, Status: "present"}, RosterEntry{StudentID: strings.ToUpper(student), Status: "late"}),
		in("2026-10-18", RosterEntry{StudentID: student, Status: "present", RecordID: &bad}),
		in("2026-10-18", RosterEntry{StudentID: student, Status: "present", Note: strings.Repeat("x", 501)}),
		in("2026-10-18", many...),
	} {
		if _, err := s.SaveRoster(context.Background(), actor, v); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	if _, err := s.SaveRoster(context.Background(), actor, in("2026-10-18", RosterEntry{StudentID: student, Status: "late", Note: "  Bus delay  "})); err != nil {
		t.Fatal(err)
	}
	if len(repo.saved) != 1 || repo.saved[0].Entries[0].Note != "Bus delay" {
		t.Fatalf("saved %+v", repo.saved)
	}
	if !errors.Is(&StaleError{StudentIDs: []string{student}}, ErrStale) {
		t.Fatal("stale error identity")
	}
}

type scanRepo struct {
	StudentRepository
	recorded []string
}

func (r *scanRepo) StudentIDByCardToken(_ context.Context, token string) (string, error) {
	if token == "AbCdEfGhIjKlMnOpQrSt-_" {
		return "55555555-5555-5555-5555-555555555555", nil
	}
	return "", ErrNotFound
}
func (r *scanRepo) RecordStudent(_ context.Context, _ string, in StudentInput, source string) (StudentRecord, error) {
	r.recorded = append(r.recorded, in.StudentID+" "+source)
	return StudentRecord{StudentID: in.StudentID}, nil
}

func TestScanAcceptsLegacyAndVerificationQR(t *testing.T) {
	const actor = "11111111-1111-1111-1111-111111111111"
	repo := &scanRepo{}
	s := NewStudentService(repo)
	ctx := context.Background()
	for _, payload := range []string{"FB-STUDENT:44444444-4444-4444-4444-444444444444", "https://school.example.test/verify/AbCdEfGhIjKlMnOpQrSt-_", " /verify/AbCdEfGhIjKlMnOpQrSt-_ "} {
		if _, err := s.Scan(ctx, actor, ScanInput{Payload: payload, AttendanceDate: "2026-10-20"}); err != nil {
			t.Fatalf("%q: %v", payload, err)
		}
	}
	want := []string{"44444444-4444-4444-4444-444444444444 qr", "55555555-5555-5555-5555-555555555555 qr", "55555555-5555-5555-5555-555555555555 qr"}
	if strings.Join(repo.recorded, ",") != strings.Join(want, ",") {
		t.Fatalf("recorded %v", repo.recorded)
	}
	if _, err := s.Scan(ctx, actor, ScanInput{Payload: "https://x.test/verify/ZZZZZZZZZZZZZZZZZZZZZZ", AttendanceDate: "2026-10-20"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown card: %v", err)
	}
	for _, bad := range []string{"FB-STUDENT:nope", "https://x.test/verify/short", "https://x.test/other/AbCdEfGhIjKlMnOpQrSt-_", "javascript:alert(1)"} {
		if _, err := s.Scan(ctx, actor, ScanInput{Payload: bad, AttendanceDate: "2026-10-20"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}
