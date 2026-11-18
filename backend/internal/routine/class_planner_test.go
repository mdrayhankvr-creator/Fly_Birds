package routine

import "testing"

func TestClassPlannerPeriodsAndLunch(t *testing.T) {
	c := ClassPlanConfig{StartTime: "08:00", EndTime: "13:30", LunchStart: "10:30", LunchMinutes: 45, PeriodMinutes: 45, TransitionMinutes: 5, Days: []int{0, 1}}
	periods, issues := buildPeriods(c, map[int]bool{5: true, 6: true})
	if len(issues) != 0 || len(periods) != 12 {
		t.Fatalf("periods=%+v issues=%+v", periods, issues)
	}
	for _, p := range periods {
		if p.Kind == "lunch" && (p.StartTime != "10:30" || p.EndTime != "11:15") {
			t.Fatalf("lunch=%+v", p)
		}
		if p.Kind == "lesson" && p.StartTime < "11:15" && p.EndTime > "10:30" {
			t.Fatalf("lesson crosses lunch: %+v", p)
		}
	}
	c.LunchMinutes = 40
	if _, issues = buildPeriods(c, nil); len(issues) == 0 {
		t.Fatal("non-45-minute lunch accepted")
	}
	c.LunchMinutes = 45
	c.Days = []int{5}
	if _, issues = buildPeriods(c, map[int]bool{5: true}); len(issues) == 0 {
		t.Fatal("weekly off day accepted")
	}
	c.Days = []int{0}
	c.EndTime = "11:00"
	if _, issues = buildPeriods(c, nil); len(issues) == 0 {
		t.Fatal("lunch beyond school day accepted")
	}
}
