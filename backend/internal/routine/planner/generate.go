package planner

import (
	"fmt"
	"sort"
)

// Generation is a deterministic, greedy placement that proposes a draft. It
// is not a promise of a feasible or optimal routine: it can fail where a
// different order would succeed. It never breaks a hard rule to make a
// paper fit; it reports what it could not place and why.

// Unplaced is a paper the generator could not place, with the rule that
// blocked it most often.
type Unplaced struct {
	PaperID string `json:"paper_id"`
	ClassID string `json:"class_id"`
	Reason  string `json:"reason"` // too_long, class_busy, students_clash, no_room, no_lab, no_days
	Message string `json:"message"`
}

// Report sums up a generation.
type Report struct {
	Placed   int        `json:"placed"`
	Kept     int        `json:"kept"` // pinned papers left where they were
	Unplaced []Unplaced `json:"unplaced"`
	Duties   int        `json:"duties"` // invigilation duties assigned
}

type roomUse struct {
	students int
	classes  map[string]bool
}

type state struct {
	p        *Plan
	ix       index
	papers   map[cell][]Paper
	rooms    map[cell]map[string]*roomUse
	order    []Room // rooms in preference order
	labCount int
}

func (st *state) use(c cell, roomID string) *roomUse {
	if st.rooms[c] == nil {
		st.rooms[c] = map[string]*roomUse{}
	}
	u := st.rooms[c][roomID]
	if u == nil {
		u = &roomUse{classes: map[string]bool{}}
		st.rooms[c][roomID] = u
	}
	return u
}

func (st *state) place(x Paper) {
	c := cell{x.Date, x.SittingID}
	st.papers[c] = append(st.papers[c], x)
	for _, sec := range st.p.sectionsSitting(st.ix, x) {
		if r := x.Rooms[sec.ID]; r != "" {
			u := st.use(c, r)
			u.students += st.p.takers(sec.ID, x.SubjectID)
			u.classes[x.ClassID] = true
		}
	}
}

// try finds rooms for a paper in a cell, or the reason it cannot go there.
func (st *state) try(x Paper, c cell, s Sitting) (map[string]string, string) {
	if x.Duration > s.Length() {
		return nil, "too_long"
	}
	for _, y := range st.papers[c] {
		if y.ClassID != x.ClassID {
			continue
		}
		if y.SubjectID != x.SubjectID && x.Optional && y.Optional {
			if st.p.both(x.ClassID, x.SubjectID, y.SubjectID) > 0 {
				return nil, "students_clash"
			}
			continue
		}
		return nil, "class_busy"
	}
	rooms := map[string]string{}
	taken := map[string]int{} // students placed in this attempt, by room
	inAttempt := map[string]bool{}
	for _, sec := range st.p.sectionsSitting(st.ix, x) {
		n := st.p.takers(sec.ID, x.SubjectID)
		chosen := ""
		// First a free room; then a room that may be shared: a hall, or a
		// room of known capacity used only by this class.
		for pass := 0; pass < 2 && chosen == ""; pass++ {
			for _, r := range st.order {
				if x.NeedsLab && r.Kind != "lab" {
					continue
				}
				u := st.rooms[c][r.ID]
				used := taken[r.ID]
				busy := inAttempt[r.ID] || (u != nil && len(u.classes) > 0)
				if u != nil {
					used += u.students
				}
				if pass == 0 && busy {
					continue
				}
				if pass == 1 {
					onlyThisClass := u == nil || len(u.classes) == 0 || (len(u.classes) == 1 && u.classes[x.ClassID])
					if !(r.Kind == "hall" || (onlyThisClass && r.Capacity != nil)) {
						continue
					}
				}
				if r.Capacity != nil && used+n > *r.Capacity {
					continue
				}
				chosen = r.ID
				break
			}
		}
		if chosen == "" {
			if x.NeedsLab {
				return nil, "no_lab"
			}
			return nil, "no_room"
		}
		rooms[sec.ID] = chosen
		taken[chosen] += n
		inAttempt[chosen] = true
	}
	return rooms, ""
}

var reasonRank = map[string]int{"no_days": 0, "too_long": 1, "no_lab": 2, "no_room": 3, "students_clash": 4, "class_busy": 5}

var reasonText = map[string]string{
	"no_days":        "No open exam day in the routine's dates for %s",
	"too_long":       "No sitting is long enough for %s",
	"no_lab":         "No lab is free for %s",
	"no_room":        "No room is free for %s",
	"students_clash": "Every free sitting clashes with another paper of %s's students",
	"class_busy":     "Every sitting already has a paper of the class for %s",
}

// Generate places every paper that is not pinned, then assigns one
// invigilator to each room in use that has none. The input plan is not
// changed.
func Generate(in *Plan) (*Plan, Report) {
	p := clone(in)
	ix := p.index()
	st := &state{p: p, ix: ix, papers: map[cell][]Paper{}, rooms: map[cell]map[string]*roomUse{}}
	for _, r := range p.Rooms {
		if r.Active {
			st.order = append(st.order, r)
			if r.Kind == "lab" {
				st.labCount++
			}
		}
	}
	pref := map[string]int{"classroom": 0, "hall": 1, "lab": 2}
	sort.SliceStable(st.order, func(i, j int) bool {
		a, b := st.order[i], st.order[j]
		if pref[a.Kind] != pref[b.Kind] {
			return pref[a.Kind] < pref[b.Kind]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	report := Report{Unplaced: []Unplaced{}}
	open := []int{}
	for i := range p.Papers {
		x := &p.Papers[i]
		if x.Pinned && x.Placed() {
			report.Kept++
			st.place(*x)
			continue
		}
		x.Date, x.SittingID, x.Rooms, x.Pinned = "", "", map[string]string{}, false
		open = append(open, i)
	}
	sittings := p.orderedSittings()
	dates := p.Dates()
	feasible := func(x Paper) int {
		if x.NeedsLab && st.labCount == 0 {
			return 0
		}
		n := 0
		for _, s := range sittings {
			if x.Duration <= s.Length() {
				n += len(dates)
			}
		}
		return n
	}
	load := map[string]int{}
	for _, x := range p.Papers {
		load[x.ClassID]++
	}
	sort.SliceStable(open, func(i, j int) bool {
		a, b := p.Papers[open[i]], p.Papers[open[j]]
		if fa, fb := feasible(a), feasible(b); fa != fb {
			return fa < fb
		}
		if load[a.ClassID] != load[b.ClassID] {
			return load[a.ClassID] > load[b.ClassID]
		}
		return paperKey(a, ix) < paperKey(b, ix)
	})
	for _, i := range open {
		x := &p.Papers[i]
		bestPenalty := -1
		var best cell
		var bestRooms map[string]string
		reasons := map[string]int{}
		for _, d := range dates {
			sameDay := 0
			for _, s := range sittings {
				for _, y := range st.papers[cell{d, s.ID}] {
					if y.ClassID == x.ClassID {
						sameDay++
						break
					}
				}
			}
			for _, s := range sittings {
				c := cell{d, s.ID}
				rooms, why := st.try(*x, c, s)
				if why != "" {
					reasons[why]++
					continue
				}
				penalty := sameDay
				if bestPenalty < 0 || penalty < bestPenalty {
					bestPenalty, best, bestRooms = penalty, c, rooms
				}
			}
			if bestPenalty == 0 {
				break
			}
		}
		if bestPenalty < 0 {
			reason := "no_days"
			for r, n := range reasons {
				if reason == "no_days" || n > reasons[reason] || (n == reasons[reason] && reasonRank[r] < reasonRank[reason]) {
					reason = r
				}
			}
			label := ix.classes[x.ClassID].Name + " " + PaperName(*x)
			report.Unplaced = append(report.Unplaced, Unplaced{PaperID: x.ID, ClassID: x.ClassID, Reason: reason, Message: fmt.Sprintf(reasonText[reason], label)})
			continue
		}
		x.Date, x.SittingID, x.Rooms = best.date, best.sitting, bestRooms
		st.place(*x)
		report.Placed++
	}
	report.Duties = assignDuties(p, st)
	return p, report
}

// assignDuties keeps the duties of rooms still in use and gives each other
// room in use one invigilator: a teacher free in that sitting, preferring
// one who does not teach a subject sat there, then the fewest duties.
func assignDuties(p *Plan, st *state) int {
	ix := st.ix
	used := map[cell]map[string]bool{}
	for c, papers := range st.papers {
		for _, x := range papers {
			for _, r := range x.Rooms {
				if used[c] == nil {
					used[c] = map[string]bool{}
				}
				used[c][r] = true
			}
		}
	}
	kept := []Duty{}
	staffed := map[cell]map[string]bool{}
	busy := map[cell]map[string]bool{}
	count := map[string]int{}
	for _, d := range p.Duties {
		c := cell{d.Date, d.SittingID}
		if !used[c][d.RoomID] || busy[c][d.EmployeeID] {
			continue
		}
		kept = append(kept, d)
		if staffed[c] == nil {
			staffed[c], busy[c] = map[string]bool{}, map[string]bool{}
		}
		staffed[c][d.RoomID], busy[c][d.EmployeeID] = true, true
		count[d.EmployeeID]++
	}
	type slot struct {
		c    cell
		room string
	}
	slots := []slot{}
	for c, rooms := range used {
		for r := range rooms {
			if !staffed[c][r] {
				slots = append(slots, slot{c, r})
			}
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.c.date != b.c.date {
			return a.c.date < b.c.date
		}
		if a.c.sitting != b.c.sitting {
			return ix.sittings[a.c.sitting].Start < ix.sittings[b.c.sitting].Start
		}
		return ix.rooms[a.room].Name < ix.rooms[b.room].Name || (ix.rooms[a.room].Name == ix.rooms[b.room].Name && a.room < b.room)
	})
	teachers := append([]Teacher(nil), p.Teachers...)
	sort.Slice(teachers, func(i, j int) bool {
		return teachers[i].Name < teachers[j].Name || (teachers[i].Name == teachers[j].Name && teachers[i].ID < teachers[j].ID)
	})
	added := 0
	for _, sl := range slots {
		own := func(t Teacher) bool {
			for _, x := range st.papers[sl.c] {
				for _, r := range x.Rooms {
					if r == sl.room && p.Teaches[t.ID][x.ClassID+"|"+x.SubjectID] {
						return true
					}
				}
			}
			return false
		}
		var pick *Teacher
		pickOwn := false
		for i := range teachers {
			t := teachers[i]
			if busy[sl.c][t.ID] {
				continue
			}
			o := own(t)
			if pick == nil || (pickOwn && !o) || (pickOwn == o && count[t.ID] < count[pick.ID]) {
				pick, pickOwn = &teachers[i], o
			}
		}
		if pick == nil {
			continue
		}
		if busy[sl.c] == nil {
			busy[sl.c] = map[string]bool{}
		}
		busy[sl.c][pick.ID] = true
		count[pick.ID]++
		kept = append(kept, Duty{Date: sl.c.date, SittingID: sl.c.sitting, RoomID: sl.room, EmployeeID: pick.ID})
		added++
	}
	p.Duties = kept
	return added
}

func clone(in *Plan) *Plan {
	out := *in
	out.Papers = make([]Paper, len(in.Papers))
	for i, x := range in.Papers {
		x.Rooms = copyMap(x.Rooms)
		out.Papers[i] = x
	}
	out.Duties = append([]Duty(nil), in.Duties...)
	return &out
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
