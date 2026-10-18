package analytics

import (
	"reflect"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPercentRoundsAndHandlesEmpty(t *testing.T) {
	if percent(1, 0) != nil {
		t.Fatal("empty denominator must be null, not 0%")
	}
	if got := *percent(2, 3); got != 66.7 {
		t.Fatalf("got %v", got)
	}
	if got := *percent(3, 3); got != 100 {
		t.Fatalf("got %v", got)
	}
}

func TestRangeDefaultsAndLimits(t *testing.T) {
	today := date("2026-10-19")
	from, to, err := Range("", "", today, 30)
	if err != nil || from != date("2026-09-20") || to != today {
		t.Fatalf("default range %v %v %v", from, to, err)
	}
	if _, _, err := Range("2026-10-10", "2026-10-01", today, 30); err == nil {
		t.Fatal("reversed range accepted")
	}
	if _, _, err := Range("2025-01-01", "2026-10-19", today, 30); err == nil {
		t.Fatal("range over 400 days accepted")
	}
	if _, _, err := Range("2025-09-15", "2026-10-18", today, 30); err != nil {
		t.Fatalf("400-day range refused: %v", err)
	}
	if _, _, err := Range("19-10-2026", "", today, 30); err == nil {
		t.Fatal("bad date accepted")
	}
}

func TestGroupingAutoAndExplicit(t *testing.T) {
	cases := []struct {
		from, to, group, want string
		bad                   bool
	}{
		{"2026-10-01", "2026-10-19", "", "day", false},
		{"2026-08-01", "2026-10-19", "", "week", false},
		{"2025-12-01", "2026-10-19", "", "month", false},
		{"2026-01-01", "2026-10-19", "day", "", true},
		{"2026-01-01", "2026-10-19", "week", "week", false},
		{"2026-10-01", "2026-10-19", "year", "", true},
	}
	for _, c := range cases {
		got, err := Grouping(c.group, date(c.from), date(c.to))
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("%v: got %q %v", c, got, err)
		}
	}
}

var bdBands = []Band{{"A+", 5}, {"A", 4}, {"A-", 3.5}, {"B", 3}, {"C", 2}, {"D", 1}, {"F", 0}}

func TestGradeForUsesSchoolBands(t *testing.T) {
	cases := []struct {
		gpa    float64
		passed bool
		want   string
	}{{5, true, "A+"}, {4.75, true, "A"}, {3.5, true, "A-"}, {3.49, true, "B"}, {1, true, "D"}, {4.5, false, "F"}, {0, false, "F"}}
	for _, c := range cases {
		if got := GradeFor(bdBands, c.gpa, c.passed); got != c.want {
			t.Errorf("GradeFor(%v,%v)=%q want %q", c.gpa, c.passed, got, c.want)
		}
	}
	custom := []Band{{"Pass", 1}, {"Distinction", 4}, {"Fail", 0}}
	if got := GradeFor(custom, 4.2, true); got != "Distinction" {
		t.Fatalf("custom bands: %q", got)
	}
}

func TestDistributionKeepsEveryBand(t *testing.T) {
	got := Distribution(bdBands, []gpaCount{{5, true, 2}, {4.25, true, 1}, {4, true, 1}, {0, false, 3}})
	want := map[string]int64{"A+": 2, "A": 2, "A-": 0, "B": 0, "C": 0, "D": 0, "F": 3}
	if len(got) != len(bdBands) || got[0].Grade != "A+" || got[len(got)-1].Grade != "F" {
		t.Fatalf("order %v", got)
	}
	for _, g := range got {
		if want[g.Grade] != g.Count {
			t.Errorf("%s: %d want %d", g.Grade, g.Count, want[g.Grade])
		}
	}
}

func TestSettingsValidate(t *testing.T) {
	if err := DefaultSettings().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []func(*Settings){
		func(s *Settings) { s.AttendanceThreshold = 0 },
		func(s *Settings) { s.AttendanceThreshold = 101 },
		func(s *Settings) { s.AttendanceWindow = 6 },
		func(s *Settings) { s.MinMarkedDays = 31 },
		func(s *Settings) { s.GPAThreshold = 5.5 },
		func(s *Settings) { s.ConsecutiveAbsences = 1 },
		func(s *Settings) { s.InactivityDays = 121 },
	}
	for i, change := range bad {
		s := DefaultSettings()
		change(&s)
		if s.Validate() == nil {
			t.Errorf("case %d accepted: %+v", i, s)
		}
	}
}

func TestClassifyRules(t *testing.T) {
	set := DefaultSettings()
	str := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	yes, no := true, false
	base := studentSignals{StudentID: "s", Marked: 20, Attended: 19, SectionDays: 15, EnrolledEarly: true, LastPresent: str("2026-10-18")}
	istart := "2026-09-29"
	cases := []struct {
		name    string
		change  func(*studentSignals)
		att, ex bool
		level   string
		reasons []string
	}{
		{"healthy", func(*studentSignals) {}, true, true, "", nil},
		{"low attendance", func(s *studentSignals) { s.Attended = 14 }, true, true, "at_risk", []string{"low_attendance"}},
		{"exactly at threshold is fine", func(s *studentSignals) { s.Attended = 15 }, true, true, "", nil},
		{"too few marks to judge", func(s *studentSignals) { s.Marked, s.Attended = 4, 0; s.AbsentStreak = 4 }, true, true, "", nil},
		{"absence run", func(s *studentSignals) { s.AbsentStreak = 5 }, true, true, "dropout", []string{"consecutive_absences"}},
		{"no presence while section marked", func(s *studentSignals) { s.LastPresent = str("2026-09-01") }, true, true, "dropout", []string{"no_recent_presence"}},
		{"never present", func(s *studentSignals) { s.LastPresent = nil; s.Marked, s.Attended, s.AbsentStreak = 6, 0, 6 }, true, true, "dropout", []string{"consecutive_absences", "no_recent_presence", "low_attendance"}},
		{"new admission is not a dropout", func(s *studentSignals) { s.LastPresent, s.EnrolledEarly, s.Marked, s.Attended = nil, false, 0, 0 }, true, true, "", nil},
		{"section not marked", func(s *studentSignals) { s.LastPresent, s.SectionDays = nil, 2 }, true, true, "", nil},
		{"failed exam", func(s *studentSignals) { s.LatestPassed, s.LatestGPA = &no, f(0) }, true, true, "at_risk", []string{"failed_exam"}},
		{"low gpa", func(s *studentSignals) { s.LatestPassed, s.LatestGPA = &yes, f(1.5) }, true, true, "at_risk", []string{"low_gpa"}},
		{"gpa at threshold", func(s *studentSignals) { s.LatestPassed, s.LatestGPA = &yes, f(2) }, true, true, "", nil},
		{"attendance module off", func(s *studentSignals) { s.AbsentStreak = 9 }, false, true, "", nil},
		{"exam module off", func(s *studentSignals) { s.LatestPassed = &no }, true, false, "", nil},
	}
	for _, c := range cases {
		s := base
		c.change(&s)
		r := Classify(s, set, c.att, c.ex, istart)
		if c.level == "" {
			if r != nil {
				t.Errorf("%s: unexpected %+v", c.name, r)
			}
			continue
		}
		if r == nil || r.Level != c.level || !reflect.DeepEqual(r.Reasons, c.reasons) {
			t.Errorf("%s: got %+v", c.name, r)
		}
	}
}
