// Package planner checks and drafts term exam routines (E8). It works on an
// in-memory plan, so the same rules run for manual placement, generation,
// submission, approval and publication, and can be tested without a
// database.
package planner

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sitting is one exam period of the day ("Morning", 10:00–13:00).
type Sitting struct {
	ID    string
	Name  string
	Start int // minutes after midnight
	End   int
	Order int
}

// Length is the sitting's length in minutes.
func (s Sitting) Length() int { return s.End - s.Start }

// Room is an exam room. Capacity nil means unknown (not checked).
type Room struct {
	ID       string
	Name     string
	Kind     string // classroom, lab or hall
	Capacity *int
	Active   bool
}

// Section is a section of a class.
type Section struct {
	ID      string
	ClassID string
	Name    string
}

// Class is a class taking part in the routine.
type Class struct {
	ID    string
	Name  string
	Order int
}

// Paper is one paper a class sits: a subject's theory, practical, viva or
// oral, and for subjects with several papers, its number.
type Paper struct {
	ID          string
	ClassID     string
	SubjectID   string
	SubjectCode string
	SubjectName string
	Kind        string
	PaperNo     int
	Duration    int
	NeedsLab    bool
	Optional    bool // the class's fourth (optional) subject
	Pinned      bool
	Date        string // YYYY-MM-DD, "" when not placed
	SittingID   string
	Rooms       map[string]string // section → room
}

// Placed reports whether the paper has a date and sitting.
func (p Paper) Placed() bool { return p.Date != "" && p.SittingID != "" }

// Duty is one invigilator in one room for one sitting.
type Duty struct {
	ID         string
	Date       string
	SittingID  string
	RoomID     string
	EmployeeID string
}

// Teacher is a member of staff who can invigilate.
type Teacher struct {
	ID   string
	Name string
}

// Plan is everything the rules need.
type Plan struct {
	From, To string            // the routine's dates, inside the term
	OffDays  map[int]bool      // weekly off days, 0 = Sunday
	Holidays map[string]string // published holidays by date → title
	Sittings []Sitting
	Rooms    []Room
	Classes  []Class
	Sections []Section
	Papers   []Paper
	Duties   []Duty
	Teachers []Teacher
	// Takers[section][subject] is the number of the section's students who
	// sit the subject.
	Takers map[string]map[string]int
	// Both[class][a+"|"+b] (a < b) is the number of the class's students who
	// take both optional subjects a and b.
	Both map[string]map[string]int
	// Teaches[employee][class+"|"+subject] marks the subjects a teacher
	// teaches.
	Teaches map[string]map[string]bool
}

// Issue is one broken rule. Hard issues block submission, approval and
// publication; soft ones only warn.
type Issue struct {
	Code      string   `json:"code"`
	Hard      bool     `json:"hard"`
	Message   string   `json:"message"`
	PaperIDs  []string `json:"paper_ids,omitempty"`
	ClassID   string   `json:"class_id,omitempty"`
	SectionID string   `json:"section_id,omitempty"`
	Date      string   `json:"date,omitempty"`
	SittingID string   `json:"sitting_id,omitempty"`
	RoomID    string   `json:"room_id,omitempty"`
	TeacherID string   `json:"teacher_id,omitempty"`
	Count     int      `json:"count,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Holiday   string   `json:"holiday,omitempty"`
}

// Completeness issues: allowed while drafting, not at submission.
var completeness = map[string]bool{"unscheduled": true, "no_room": true}

// Placement reports whether an issue makes a placement impossible (every
// hard issue except a paper still waiting for its date or rooms).
func (i Issue) Placement() bool { return i.Hard && !completeness[i.Code] }

type index struct {
	sittings map[string]Sitting
	rooms    map[string]Room
	classes  map[string]Class
	sections map[string]Section
	teachers map[string]Teacher
	papers   map[string]Paper
	byClass  map[string][]Section
}

func (p *Plan) index() index {
	ix := index{sittings: map[string]Sitting{}, rooms: map[string]Room{}, classes: map[string]Class{}, sections: map[string]Section{}, teachers: map[string]Teacher{}, papers: map[string]Paper{}, byClass: map[string][]Section{}}
	for _, s := range p.Sittings {
		ix.sittings[s.ID] = s
	}
	for _, r := range p.Rooms {
		ix.rooms[r.ID] = r
	}
	for _, c := range p.Classes {
		ix.classes[c.ID] = c
	}
	for _, s := range p.Sections {
		ix.sections[s.ID] = s
		ix.byClass[s.ClassID] = append(ix.byClass[s.ClassID], s)
	}
	for k := range ix.byClass {
		sort.Slice(ix.byClass[k], func(i, j int) bool {
			a, b := ix.byClass[k][i], ix.byClass[k][j]
			return a.Name < b.Name || (a.Name == b.Name && a.ID < b.ID)
		})
	}
	for _, t := range p.Teachers {
		ix.teachers[t.ID] = t
	}
	for _, x := range p.Papers {
		ix.papers[x.ID] = x
	}
	return ix
}

// takers is how many of a section's students sit a paper.
func (p *Plan) takers(sectionID, subjectID string) int { return p.Takers[sectionID][subjectID] }

// both is how many students of a class take both subjects.
func (p *Plan) both(classID, a, b string) int {
	if a > b {
		a, b = b, a
	}
	return p.Both[classID][a+"|"+b]
}

// SectionsSitting lists the sections of a paper's class that have students
// sitting it (they need a room).
func (p *Plan) SectionsSitting(x Paper) []Section {
	ix := p.index()
	return p.sectionsSitting(ix, x)
}

func (p *Plan) sectionsSitting(ix index, x Paper) []Section {
	out := []Section{}
	for _, s := range ix.byClass[x.ClassID] {
		if p.takers(s.ID, x.SubjectID) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// DayClosed says why a date cannot hold a paper: "weekly_off", "holiday"
// (with its title) or "outside_term"; "" when it can.
func (p *Plan) DayClosed(date string) (string, string) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil || date < p.From || date > p.To {
		return "outside_term", ""
	}
	if title, ok := p.Holidays[date]; ok {
		return "holiday", title
	}
	if p.OffDays[int(d.Weekday())] {
		return "weekly_off", ""
	}
	return "", ""
}

// Dates lists the open dates of the plan, in order.
func (p *Plan) Dates() []string {
	out := []string{}
	from, err1 := time.Parse(time.DateOnly, p.From)
	to, err2 := time.Parse(time.DateOnly, p.To)
	if err1 != nil || err2 != nil {
		return out
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if why, _ := p.DayClosed(d.Format(time.DateOnly)); why == "" {
			out = append(out, d.Format(time.DateOnly))
		}
	}
	return out
}

func (p *Plan) orderedSittings() []Sitting {
	out := append([]Sitting(nil), p.Sittings...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func clock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// PaperName is a paper's English label ("Physics theory", "English 2nd paper").
func PaperName(x Paper) string {
	name := x.SubjectName
	if x.PaperNo > 1 {
		name += fmt.Sprintf(" paper %d", x.PaperNo)
	}
	if x.Kind != "theory" {
		name += " " + x.Kind
	}
	return name
}

type cell struct{ date, sitting string }

// Validate checks a plan. With final set, every paper must have a date and
// a room for each section that sits it.
func Validate(p *Plan, final bool) []Issue {
	ix := p.index()
	issues := []Issue{}
	add := func(i Issue) { issues = append(issues, i) }
	byCell := map[cell][]Paper{}
	for _, x := range sortedPapers(p.Papers, ix) {
		className := ix.classes[x.ClassID].Name
		if !x.Placed() {
			if final {
				add(Issue{Code: "unscheduled", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, Message: fmt.Sprintf("%s %s has no date", className, PaperName(x))})
			}
			continue
		}
		s, ok := ix.sittings[x.SittingID]
		if !ok {
			add(Issue{Code: "unscheduled", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, Message: fmt.Sprintf("%s %s has no sitting", className, PaperName(x))})
			continue
		}
		byCell[cell{x.Date, x.SittingID}] = append(byCell[cell{x.Date, x.SittingID}], x)
		switch why, title := p.DayClosed(x.Date); why {
		case "holiday":
			add(Issue{Code: "holiday", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, Date: x.Date, Holiday: title, Message: fmt.Sprintf("%s is %s (holiday)", x.Date, title)})
		case "weekly_off":
			add(Issue{Code: "weekly_off", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, Date: x.Date, Message: fmt.Sprintf("%s is a weekly off day", x.Date)})
		case "outside_term":
			add(Issue{Code: "outside_term", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, Date: x.Date, Message: fmt.Sprintf("%s is outside the routine's dates (%s to %s)", x.Date, p.From, p.To)})
		}
		if x.Duration > s.Length() {
			add(Issue{Code: "too_long", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, SittingID: s.ID, Count: x.Duration, Limit: s.Length(), Message: fmt.Sprintf("%s %s needs %d min; %s is %d min", className, PaperName(x), x.Duration, s.Name, s.Length())})
		}
		for _, sec := range p.sectionsSitting(ix, x) {
			roomID := x.Rooms[sec.ID]
			if roomID == "" {
				if final {
					add(Issue{Code: "no_room", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, SectionID: sec.ID, Message: fmt.Sprintf("%s %s: section %s has no room", className, PaperName(x), sec.Name)})
				}
				continue
			}
			room, ok := ix.rooms[roomID]
			if !ok || !room.Active {
				add(Issue{Code: "room_inactive", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, SectionID: sec.ID, RoomID: roomID, Message: fmt.Sprintf("%s %s: section %s's room is not in use", className, PaperName(x), sec.Name)})
				continue
			}
			if x.NeedsLab && room.Kind != "lab" {
				add(Issue{Code: "needs_lab", Hard: true, PaperIDs: []string{x.ID}, ClassID: x.ClassID, SectionID: sec.ID, RoomID: roomID, Message: fmt.Sprintf("%s %s must be in a lab; %s is not one", className, PaperName(x), room.Name)})
			}
		}
	}
	cells := make([]cell, 0, len(byCell))
	for c := range byCell {
		cells = append(cells, c)
	}
	order := map[string]int{}
	for i, s := range p.orderedSittings() {
		order[s.ID] = i
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].date != cells[j].date {
			return cells[i].date < cells[j].date
		}
		return order[cells[i].sitting] < order[cells[j].sitting]
	})
	for _, c := range cells {
		papers := byCell[c]
		// Students: two papers of one class at once.
		for i := 0; i < len(papers); i++ {
			for j := i + 1; j < len(papers); j++ {
				a, b := papers[i], papers[j]
				if a.ClassID != b.ClassID {
					continue
				}
				className := ix.classes[a.ClassID].Name
				if a.SubjectID != b.SubjectID && a.Optional && b.Optional {
					if n := p.both(a.ClassID, a.SubjectID, b.SubjectID); n > 0 {
						add(Issue{Code: "students_clash", Hard: true, PaperIDs: []string{a.ID, b.ID}, ClassID: a.ClassID, Date: c.date, SittingID: c.sitting, Count: n, Message: fmt.Sprintf("%d %s students take both %s and %s", n, className, PaperName(a), PaperName(b))})
					}
					continue
				}
				add(Issue{Code: "class_clash", Hard: true, PaperIDs: []string{a.ID, b.ID}, ClassID: a.ClassID, Date: c.date, SittingID: c.sitting, Message: fmt.Sprintf("%s already has %s this sitting (%s)", className, PaperName(a), PaperName(b))})
			}
		}
		// Rooms: capacity, and classes sharing a room that is not a hall.
		type use struct {
			students int
			classes  map[string]bool
			papers   []string
		}
		uses := map[string]*use{}
		roomOrder := []string{}
		for _, x := range papers {
			for _, sec := range p.sectionsSitting(ix, x) {
				roomID := x.Rooms[sec.ID]
				if roomID == "" {
					continue
				}
				u := uses[roomID]
				if u == nil {
					u = &use{classes: map[string]bool{}}
					uses[roomID] = u
					roomOrder = append(roomOrder, roomID)
				}
				u.students += p.takers(sec.ID, x.SubjectID)
				u.classes[x.ClassID] = true
				if !contains(u.papers, x.ID) {
					u.papers = append(u.papers, x.ID)
				}
			}
		}
		sort.Slice(roomOrder, func(i, j int) bool { return ix.rooms[roomOrder[i]].Name < ix.rooms[roomOrder[j]].Name })
		for _, roomID := range roomOrder {
			u, room := uses[roomID], ix.rooms[roomID]
			if room.Capacity != nil && u.students > *room.Capacity {
				add(Issue{Code: "room_capacity", Hard: true, PaperIDs: u.papers, RoomID: roomID, Date: c.date, SittingID: c.sitting, Count: u.students, Limit: *room.Capacity, Message: fmt.Sprintf("%s holds %d; %d assigned", room.Name, *room.Capacity, u.students)})
			}
			if len(u.classes) > 1 && room.Kind != "hall" {
				add(Issue{Code: "room_shared", Hard: true, PaperIDs: u.papers, RoomID: roomID, Date: c.date, SittingID: c.sitting, Count: len(u.classes), Message: fmt.Sprintf("%s holds %d classes but is not a hall", room.Name, len(u.classes))})
			}
		}
		// Invigilators.
		seen := map[string]string{}
		staffed := map[string]bool{}
		for _, d := range sortedDuties(p.Duties, ix) {
			if d.Date != c.date || d.SittingID != c.sitting {
				continue
			}
			staffed[d.RoomID] = true
			if other, ok := seen[d.EmployeeID]; ok && other != d.RoomID {
				add(Issue{Code: "invigilator_clash", Hard: true, TeacherID: d.EmployeeID, RoomID: d.RoomID, Date: c.date, SittingID: c.sitting, Message: fmt.Sprintf("%s is in %s and %s", ix.teachers[d.EmployeeID].Name, ix.rooms[other].Name, ix.rooms[d.RoomID].Name)})
				continue
			}
			seen[d.EmployeeID] = d.RoomID
			if u, ok := uses[d.RoomID]; ok {
				for _, pid := range u.papers {
					x := ix.papers[pid]
					if p.Teaches[d.EmployeeID][x.ClassID+"|"+x.SubjectID] {
						add(Issue{Code: "own_subject", TeacherID: d.EmployeeID, RoomID: d.RoomID, PaperIDs: []string{x.ID}, Date: c.date, SittingID: c.sitting, Message: fmt.Sprintf("%s invigilates their own subject %s", ix.teachers[d.EmployeeID].Name, PaperName(x))})
					}
				}
			}
		}
		for _, roomID := range roomOrder {
			if !staffed[roomID] {
				add(Issue{Code: "no_invigilator", RoomID: roomID, Date: c.date, SittingID: c.sitting, PaperIDs: uses[roomID].papers, Message: fmt.Sprintf("%s has no invigilator", ix.rooms[roomID].Name)})
			}
		}
	}
	// Duties in a room that holds no paper, or on an unknown sitting.
	for _, d := range sortedDuties(p.Duties, ix) {
		used := false
		for _, x := range byCell[cell{d.Date, d.SittingID}] {
			for _, r := range x.Rooms {
				used = used || r == d.RoomID
			}
		}
		if !used {
			add(Issue{Code: "duty_unused", TeacherID: d.EmployeeID, RoomID: d.RoomID, Date: d.Date, SittingID: d.SittingID, Message: fmt.Sprintf("%s is on duty in %s on %s, which holds no paper then", ix.teachers[d.EmployeeID].Name, ix.rooms[d.RoomID].Name, d.Date)})
		}
	}
	// Soft: more than one paper a day for a class.
	perDay := map[string]map[string]bool{}
	for _, x := range p.Papers {
		if !x.Placed() {
			continue
		}
		k := x.ClassID + "|" + x.Date
		if perDay[k] == nil {
			perDay[k] = map[string]bool{}
		}
		perDay[k][x.SittingID] = true
	}
	keys := make([]string, 0, len(perDay))
	for k := range perDay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(perDay[k]) > 1 {
			parts := strings.SplitN(k, "|", 2)
			add(Issue{Code: "two_a_day", ClassID: parts[0], Date: parts[1], Count: len(perDay[k]), Message: fmt.Sprintf("%s has %d sittings on %s", ix.classes[parts[0]].Name, len(perDay[k]), parts[1])})
		}
	}
	return issues
}

// Hard filters the hard issues.
func Hard(issues []Issue) []Issue {
	out := []Issue{}
	for _, i := range issues {
		if i.Hard {
			out = append(out, i)
		}
	}
	return out
}

// Involving filters the placement issues that name a paper.
func Involving(issues []Issue, paperID string) []Issue {
	out := []Issue{}
	for _, i := range issues {
		if i.Placement() && contains(i.PaperIDs, paperID) {
			out = append(out, i)
		}
	}
	return out
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func sortedPapers(xs []Paper, ix index) []Paper {
	out := append([]Paper(nil), xs...)
	sort.Slice(out, func(i, j int) bool { return paperKey(out[i], ix) < paperKey(out[j], ix) })
	return out
}

var kindOrder = map[string]int{"theory": 0, "practical": 1, "viva": 2, "oral": 3}

func paperKey(x Paper, ix index) string {
	c := ix.classes[x.ClassID]
	return fmt.Sprintf("%08d|%s|%s|%s|%d|%d|%s", c.Order, c.Name, x.ClassID, x.SubjectCode, x.PaperNo, kindOrder[x.Kind], x.ID)
}

func sortedDuties(xs []Duty, ix index) []Duty {
	out := append([]Duty(nil), xs...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.SittingID != b.SittingID {
			return ix.sittings[a.SittingID].Start < ix.sittings[b.SittingID].Start
		}
		if ix.rooms[a.RoomID].Name != ix.rooms[b.RoomID].Name {
			return ix.rooms[a.RoomID].Name < ix.rooms[b.RoomID].Name
		}
		return a.EmployeeID < b.EmployeeID
	})
	return out
}
