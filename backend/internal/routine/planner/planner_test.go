package planner

import (
	"fmt"
	"reflect"
	"testing"
)

func ptr(n int) *int { return &n }

// lifecycleSchool is the spec's fixture: 10 classes, 63 papers, 2 sittings
// and 15 open days (Fridays off, one holiday).
func lifecycleSchool() *Plan {
	p := &Plan{
		From: "2027-01-03", To: "2027-01-20",
		OffDays:  map[int]bool{5: true},
		Holidays: map[string]string{"2027-01-12": "Shab-e-Barat"},
		Sittings: []Sitting{{ID: "s-am", Name: "Morning", Start: 600, End: 780}, {ID: "s-pm", Name: "Afternoon", Start: 840, End: 960, Order: 1}},
		Takers:   map[string]map[string]int{},
		Both:     map[string]map[string]int{},
		Teaches:  map[string]map[string]bool{},
	}
	for i := 1; i <= 12; i++ {
		p.Rooms = append(p.Rooms, Room{ID: fmt.Sprintf("r%02d", i), Name: fmt.Sprintf("Room %d", 100+i), Kind: "classroom", Capacity: ptr(45), Active: true})
	}
	p.Rooms = append(p.Rooms, Room{ID: "hall", Name: "Hall", Kind: "hall", Capacity: ptr(120), Active: true},
		Room{ID: "lab1", Name: "Lab 1", Kind: "lab", Capacity: ptr(45), Active: true}, Room{ID: "lab2", Name: "Lab 2", Kind: "lab", Capacity: ptr(45), Active: true})
	for i := 1; i <= 20; i++ {
		p.Teachers = append(p.Teachers, Teacher{ID: fmt.Sprintf("t%02d", i), Name: fmt.Sprintf("Teacher %02d", i)})
	}
	n := 0
	paper := func(class, subject string, kind string, no, dur int, lab, optional bool) {
		n++
		p.Papers = append(p.Papers, Paper{ID: fmt.Sprintf("p%03d", n), ClassID: class, SubjectID: subject, SubjectCode: subject, SubjectName: subject, Kind: kind, PaperNo: no, Duration: dur, NeedsLab: lab, Optional: optional})
	}
	for c := 1; c <= 10; c++ {
		class := fmt.Sprintf("c%02d", c)
		p.Classes = append(p.Classes, Class{ID: class, Name: fmt.Sprintf("Class %d", c), Order: c})
		sections := []string{"A"}
		if c > 5 {
			sections = []string{"A", "B"}
		}
		subjects := []string{}
		switch {
		case c <= 3:
			subjects = []string{"BAN", "ENG", "MAT", "SCI", "REL"}
		case c <= 8:
			subjects = []string{"BAN", "ENG", "MAT", "SCI", "SOC", "REL"}
		}
		for _, s := range sections {
			id := class + s
			p.Sections = append(p.Sections, Section{ID: id, ClassID: class, Name: s})
			p.Takers[id] = map[string]int{}
			for _, sub := range []string{"BAN", "ENG", "MAT", "SCI", "SOC", "REL", "PHY"} {
				p.Takers[id][sub] = 38
			}
			p.Takers[id]["HMA"], p.Takers[id]["AGR"] = 20, 18
		}
		for _, sub := range subjects {
			paper(class, sub, "theory", 1, 120, false, false)
		}
		if c >= 9 {
			paper(class, "BAN", "theory", 1, 180, false, false)
			paper(class, "BAN", "theory", 2, 180, false, false)
			paper(class, "ENG", "theory", 1, 180, false, false)
			paper(class, "ENG", "theory", 2, 180, false, false)
			paper(class, "MAT", "theory", 1, 180, false, false)
			paper(class, "PHY", "theory", 1, 180, false, false)
			paper(class, "PHY", "practical", 1, 120, true, false)
			paper(class, "HMA", "theory", 1, 180, false, true)
			paper(class, "AGR", "theory", 1, 180, false, true)
			p.Both[class] = map[string]int{"AGR|HMA": 0}
		}
	}
	p.Teaches["t01"] = map[string]bool{"c01|BAN": true}
	return p
}

func TestLifecycleSchoolIsPlacedFully(t *testing.T) {
	p := lifecycleSchool()
	if len(p.Papers) != 63 || len(p.Classes) != 10 || len(p.Dates()) != 15 {
		t.Fatalf("fixture: %d papers, %d classes, %d days", len(p.Papers), len(p.Classes), len(p.Dates()))
	}
	out, report := Generate(p)
	if report.Placed != 63 || len(report.Unplaced) != 0 {
		t.Fatalf("placed %d, unplaced %+v", report.Placed, report.Unplaced)
	}
	if hard := Hard(Validate(out, true)); len(hard) != 0 {
		t.Fatalf("generated routine breaks hard rules: %+v", hard)
	}
	for _, i := range Validate(out, true) {
		if i.Code == "no_invigilator" || i.Code == "own_subject" {
			t.Fatalf("invigilation left incomplete: %+v", i)
		}
	}
	if report.Duties == 0 || len(out.Duties) != report.Duties {
		t.Fatalf("duties %d of %d", report.Duties, len(out.Duties))
	}
	for _, x := range out.Papers {
		if why, _ := out.DayClosed(x.Date); why != "" {
			t.Fatalf("%s placed on a closed day %s (%s)", x.ID, x.Date, why)
		}
		if x.NeedsLab {
			for _, r := range x.Rooms {
				if r != "lab1" && r != "lab2" {
					t.Fatalf("practical in %s", r)
				}
			}
		}
	}
	// The input is untouched.
	for _, x := range p.Papers {
		if x.Placed() {
			t.Fatal("Generate changed its input")
		}
	}
}

func TestGenerationIsDeterministic(t *testing.T) {
	a, ra := Generate(lifecycleSchool())
	for i := 0; i < 5; i++ {
		b, rb := Generate(lifecycleSchool())
		if !reflect.DeepEqual(a.Papers, b.Papers) || !reflect.DeepEqual(a.Duties, b.Duties) || !reflect.DeepEqual(ra, rb) {
			t.Fatal("same input, different output")
		}
	}
	// Regenerating a generated routine gives the same routine.
	c, _ := Generate(a)
	if !reflect.DeepEqual(a.Papers, c.Papers) {
		t.Fatal("regeneration moved papers")
	}
}

func TestGenerationKeepsPinnedPapers(t *testing.T) {
	p := lifecycleSchool()
	p.Papers[0].Date, p.Papers[0].SittingID, p.Papers[0].Pinned = "2027-01-20", "s-pm", true
	p.Papers[0].Rooms = map[string]string{"c01A": "r12"}
	out, report := Generate(p)
	if report.Kept != 1 || out.Papers[0].Date != "2027-01-20" || out.Papers[0].SittingID != "s-pm" || out.Papers[0].Rooms["c01A"] != "r12" {
		t.Fatalf("pinned paper moved: %+v %+v", out.Papers[0], report)
	}
	if hard := Hard(Validate(out, true)); len(hard) != 0 {
		t.Fatalf("hard issues: %+v", hard)
	}
}

func TestInfeasibleReportsReasons(t *testing.T) {
	p := &Plan{
		From: "2027-02-01", To: "2027-02-01", // a Monday
		Sittings: []Sitting{{ID: "s", Name: "Morning", Start: 600, End: 720}},
		Rooms:    []Room{{ID: "r1", Name: "Room 1", Kind: "classroom", Capacity: ptr(40), Active: true}},
		Classes:  []Class{{ID: "c", Name: "Class 9"}},
		Sections: []Section{{ID: "cA", ClassID: "c", Name: "A"}},
		Takers:   map[string]map[string]int{"cA": {"BAN": 30, "ENG": 30, "PHY": 30, "CHE": 30}},
		Papers: []Paper{
			{ID: "a", ClassID: "c", SubjectID: "BAN", SubjectCode: "BAN", SubjectName: "Bangla", Kind: "theory", PaperNo: 1, Duration: 120},
			{ID: "b", ClassID: "c", SubjectID: "ENG", SubjectCode: "ENG", SubjectName: "English", Kind: "theory", PaperNo: 1, Duration: 120},
			{ID: "c", ClassID: "c", SubjectID: "PHY", SubjectCode: "PHY", SubjectName: "Physics", Kind: "theory", PaperNo: 1, Duration: 180},
			{ID: "d", ClassID: "c", SubjectID: "CHE", SubjectCode: "CHE", SubjectName: "Chemistry", Kind: "practical", PaperNo: 1, Duration: 90, NeedsLab: true},
		},
	}
	out, report := Generate(p)
	got := map[string]string{}
	for _, u := range report.Unplaced {
		got[u.PaperID] = u.Reason
	}
	want := map[string]string{"b": "class_busy", "c": "too_long", "d": "no_lab"}
	if report.Placed != 1 || !reflect.DeepEqual(got, want) {
		t.Fatalf("placed %d, reasons %v", report.Placed, got)
	}
	for _, u := range report.Unplaced {
		if u.PaperID == "c" && u.Message != "No sitting is long enough for Class 9 Physics" {
			t.Fatalf("message %q", u.Message)
		}
	}
	if hard := Hard(Validate(out, false)); len(hard) != 0 {
		t.Fatalf("draft has hard issues: %+v", hard)
	}
	if codes := codesOf(Hard(Validate(out, true))); !reflect.DeepEqual(codes, []string{"unscheduled", "unscheduled", "unscheduled"}) {
		t.Fatalf("final check: %v", codes)
	}
	// No open day at all.
	p.OffDays = map[int]bool{1: true}
	_, report = Generate(p)
	for _, u := range report.Unplaced {
		if u.Reason != "no_days" {
			t.Fatalf("closed window: %+v", u)
		}
	}
}

func codesOf(xs []Issue) []string {
	out := []string{}
	for _, i := range xs {
		out = append(out, i.Code)
	}
	return out
}

func has(xs []Issue, code string) *Issue {
	for i := range xs {
		if xs[i].Code == code {
			return &xs[i]
		}
	}
	return nil
}

func TestValidatorRules(t *testing.T) {
	base := func() *Plan {
		return &Plan{
			From: "2027-03-01", To: "2027-03-10",
			OffDays:  map[int]bool{5: true},
			Holidays: map[string]string{"2027-03-08": "Holiday X"},
			Sittings: []Sitting{{ID: "am", Name: "Morning", Start: 600, End: 750}, {ID: "pm", Name: "Afternoon", Start: 840, End: 960}},
			Rooms: []Room{{ID: "r1", Name: "Room 204", Kind: "classroom", Capacity: ptr(40), Active: true}, {ID: "r2", Name: "Room 101", Kind: "classroom", Active: true},
				{ID: "h", Name: "Hall", Kind: "hall", Capacity: ptr(100), Active: true}, {ID: "lab", Name: "Lab", Kind: "lab", Active: true}, {ID: "old", Name: "Old", Kind: "classroom", Active: false}},
			Classes:  []Class{{ID: "c9", Name: "Class 9"}, {ID: "c7", Name: "Class 7"}},
			Sections: []Section{{ID: "9A", ClassID: "c9", Name: "A"}, {ID: "9B", ClassID: "c9", Name: "B"}, {ID: "7A", ClassID: "c7", Name: "A"}},
			Takers:   map[string]map[string]int{"9A": {"PHY": 39, "HMA": 14, "BIO": 30, "AGR": 10}, "9B": {"PHY": 39, "HMA": 0, "BIO": 39}, "7A": {"MAT": 30}},
			Both:     map[string]map[string]int{"c9": {"BIO|HMA": 14, "AGR|HMA": 0}},
			Teachers: []Teacher{{ID: "k", Name: "Mr Karim"}, {ID: "s", Name: "Ms Sara"}},
			Teaches:  map[string]map[string]bool{"k": {"c7|MAT": true}},
		}
	}
	at := func(id, class, subject, kind string, dur int, date, sitting string, rooms map[string]string) Paper {
		return Paper{ID: id, ClassID: class, SubjectID: subject, SubjectCode: subject, SubjectName: subject, Kind: kind, PaperNo: 1, Duration: dur, Date: date, SittingID: sitting, Rooms: rooms}
	}
	t.Run("calendar", func(t *testing.T) {
		p := base()
		p.Papers = []Paper{
			at("x1", "c7", "MAT", "theory", 120, "2027-03-08", "am", map[string]string{"7A": "r2"}),    // holiday
			at("x2", "c7", "MAT", "practical", 120, "2027-03-05", "am", map[string]string{"7A": "r2"}), // Friday
			at("x3", "c7", "MAT", "viva", 30, "2027-03-11", "am", map[string]string{"7A": "r2"}),       // after the window
		}
		is := Validate(p, true)
		if h := has(is, "holiday"); h == nil || h.Holiday != "Holiday X" || h.Message != "2027-03-08 is Holiday X (holiday)" {
			t.Fatalf("holiday: %+v", is)
		}
		if has(is, "weekly_off") == nil || has(is, "outside_term") == nil {
			t.Fatalf("calendar: %v", codesOf(is))
		}
	})
	t.Run("students and classes", func(t *testing.T) {
		p := base()
		p.Papers = []Paper{
			at("phy", "c9", "PHY", "theory", 120, "2027-03-01", "am", map[string]string{"9A": "r2", "9B": "h"}),
			at("phyp", "c9", "PHY", "practical", 60, "2027-03-01", "am", map[string]string{"9A": "lab", "9B": "lab"}),
		}
		if h := has(Validate(p, true), "class_clash"); h == nil || len(h.PaperIDs) != 2 {
			t.Fatal("same-class clash not found")
		}
		hma := at("hma", "c9", "HMA", "theory", 120, "2027-03-02", "am", map[string]string{"9A": "r2"})
		bio := at("bio", "c9", "BIO", "theory", 120, "2027-03-02", "am", map[string]string{"9A": "h", "9B": "h"})
		agr := at("agr", "c9", "AGR", "theory", 120, "2027-03-02", "am", map[string]string{"9A": "r1"})
		hma.Optional, bio.Optional, agr.Optional = true, true, true
		p.Papers = []Paper{hma, bio}
		if h := has(Validate(p, true), "students_clash"); h == nil || h.Count != 14 || h.Message != "14 Class 9 students take both BIO and HMA" {
			t.Fatalf("students clash: %+v", Validate(p, true))
		}
		p.Papers = []Paper{hma, agr} // alternatives nobody takes together
		if hard := Hard(Validate(p, true)); len(hard) != 0 {
			t.Fatalf("alternatives flagged: %+v", hard)
		}
		// Different classes at once are fine.
		p.Papers = []Paper{at("m7", "c7", "MAT", "theory", 120, "2027-03-02", "am", map[string]string{"7A": "r1"}), hma}
		if hard := Hard(Validate(p, true)); len(hard) != 0 {
			t.Fatalf("two classes flagged: %+v", hard)
		}
	})
	t.Run("rooms", func(t *testing.T) {
		p := base()
		p.Papers = []Paper{at("phy", "c9", "PHY", "theory", 120, "2027-03-01", "am", map[string]string{"9A": "r1", "9B": "r1"})}
		if h := has(Validate(p, true), "room_capacity"); h == nil || h.Message != "Room 204 holds 40; 78 assigned" {
			t.Fatalf("capacity: %+v", Validate(p, true))
		}
		p.Papers = []Paper{at("phy", "c9", "PHY", "theory", 120, "2027-03-01", "am", map[string]string{"9A": "r2", "9B": "h"}), at("m7", "c7", "MAT", "theory", 120, "2027-03-01", "am", map[string]string{"7A": "r2"})}
		if h := has(Validate(p, true), "room_shared"); h == nil {
			t.Fatal("two classes in a classroom")
		}
		p.Papers[0].Rooms["9A"], p.Papers[1].Rooms["7A"] = "h", "h" // 39+39+30 = 108 > 100
		is := Validate(p, true)
		if has(is, "room_shared") != nil || has(is, "room_capacity") == nil {
			t.Fatalf("hall: %v", codesOf(is))
		}
		p.Papers = []Paper{at("pp", "c9", "PHY", "practical", 120, "2027-03-01", "am", map[string]string{"9A": "lab", "9B": "r2"})}
		p.Papers[0].NeedsLab = true
		if h := has(Validate(p, true), "needs_lab"); h == nil || h.SectionID != "9B" {
			t.Fatal("lab rule")
		}
		p.Papers = []Paper{at("phy", "c9", "PHY", "theory", 120, "2027-03-01", "am", map[string]string{"9A": "old"})}
		is = Validate(p, true)
		if has(is, "room_inactive") == nil || has(is, "no_room") == nil || has(Validate(p, false), "no_room") != nil {
			t.Fatalf("rooms: %v", codesOf(is))
		}
	})
	t.Run("sittings and invigilators", func(t *testing.T) {
		p := base()
		p.Papers = []Paper{
			at("phy", "c9", "PHY", "theory", 180, "2027-03-01", "am", map[string]string{"9A": "r2", "9B": "h"}),
			at("m7", "c7", "MAT", "theory", 120, "2027-03-01", "am", map[string]string{"7A": "r1"}),
		}
		p.Duties = []Duty{{Date: "2027-03-01", SittingID: "am", RoomID: "r2", EmployeeID: "k"}, {Date: "2027-03-01", SittingID: "am", RoomID: "r1", EmployeeID: "k"}, {Date: "2027-03-02", SittingID: "am", RoomID: "r1", EmployeeID: "s"}}
		is := Validate(p, true)
		if h := has(is, "too_long"); h == nil || h.Message != "Class 9 PHY needs 180 min; Morning is 150 min" {
			t.Fatalf("too long: %+v", is)
		}
		if h := has(is, "invigilator_clash"); h == nil || h.Message != "Mr Karim is in Room 101 and Room 204" {
			t.Fatalf("invigilator: %+v", is)
		}
		if has(is, "no_invigilator") == nil || has(is, "duty_unused") == nil {
			t.Fatalf("soft: %v", codesOf(is))
		}
		for _, code := range []string{"no_invigilator", "duty_unused", "own_subject", "two_a_day"} {
			if i := has(is, code); i != nil && i.Hard {
				t.Fatalf("%s should be soft", code)
			}
		}
		p.Duties = []Duty{{Date: "2027-03-01", SittingID: "am", RoomID: "r1", EmployeeID: "k"}}
		if has(Validate(p, true), "own_subject") == nil {
			t.Fatal("own subject")
		}
		p.Papers = append(p.Papers, at("m7b", "c7", "MAT", "practical", 60, "2027-03-01", "pm", map[string]string{"7A": "r1"}))
		if has(Validate(p, true), "two_a_day") == nil {
			t.Fatal("two a day")
		}
		if got := Involving(Validate(p, true), "phy"); len(got) != 1 || got[0].Code != "too_long" {
			t.Fatalf("involving: %+v", got)
		}
	})
}
