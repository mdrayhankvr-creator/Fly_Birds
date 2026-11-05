package documents

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Exam routine documents (E8): the routine of each class, the use of each
// room, and the invigilation duties. The routine package gathers the
// published (or draft) routine; this file only words and lays it out in
// English or Bangla with the school's header, through the same Typst
// renderer as result documents. Routines are small, so they are rendered
// on request.

// Name is a name in both languages.
type Name struct {
	EN string `json:"en"`
	BN string `json:"bn"`
}

// RoutineDoc is one routine document.
type RoutineDoc struct {
	Kind         string // class, room or invigilation
	Language     string // en or bn
	Term         string
	Session      string
	Version      int
	Status       string // draft, submitted, approved, published, superseded
	PublishedAt  string // RFC 3339, or ""
	Reason       string // why this version amends the previous one
	Instructions string
	From, To     string // YYYY-MM-DD
	Holidays     []RoutineHoliday
	Pages        []RoutinePage
}

// RoutineHoliday is a closed date within the routine's dates.
type RoutineHoliday struct {
	Date  string
	Title Name
}

// RoutinePage is one page group: a class, a room, or the duty roster.
type RoutinePage struct {
	Title Name
	Note  Name // e.g. the room's kind and capacity
	Rows  []RoutineRow
}

// RoutineRow is one line: a paper (class routine), or a room in a sitting
// (room and invigilation documents).
type RoutineRow struct {
	Date         string
	Sitting      Name
	Start, End   int
	Room         Name
	Papers       []RoutinePaper
	Students     int
	Invigilators []Name
}

// RoutinePaper is a paper with the rooms its sections sit in.
type RoutinePaper struct {
	Class    string
	Sections string // "A, B" (room documents)
	Subject  Name
	Code     string
	Kind     string // theory, practical, viva, oral
	PaperNo  int
	Duration int
	Optional bool
	Rooms    []RoutineRoomUse
}

// RoutineRoomUse is where one section sits a paper.
type RoutineRoomUse struct {
	Section string
	Room    Name
}

var routineLabels = map[string][2]string{
	"title":           {"Examination Routine", "পরীক্ষার রুটিন"},
	"rooms_title":     {"Exam room schedule", "পরীক্ষা কক্ষের সূচি"},
	"duties_title":    {"Invigilation duties", "পরিদর্শনের দায়িত্ব"},
	"date":            {"Date", "তারিখ"},
	"sitting":         {"Sitting and time", "পালা ও সময়"},
	"subject":         {"Subject and paper", "বিষয় ও পত্র"},
	"duration":        {"Duration", "সময়কাল"},
	"rooms":           {"Rooms", "কক্ষ"},
	"room":            {"Room", "কক্ষ"},
	"classes":         {"Classes and papers", "শ্রেণি ও পত্র"},
	"students":        {"Students", "শিক্ষার্থী"},
	"invigilators":    {"Invigilators", "পরিদর্শক"},
	"version":         {"Version %s", "সংস্করণ %s"},
	"published":       {"Published %s", "প্রকাশ %s"},
	"amended":         {"Amended: %s", "সংশোধন: %s"},
	"holidays":        {"Holidays in this period: %s", "এই সময়ের ছুটি: %s"},
	"instructions":    {"Instructions", "নির্দেশনা"},
	"draft":           {"DRAFT — NOT PUBLISHED", "খসড়া — প্রকাশিত নয়"},
	"superseded":      {"SUPERSEDED BY A LATER VERSION", "পরবর্তী সংস্করণে প্রতিস্থাপিত"},
	"page":            {"Page", "পৃষ্ঠা"},
	"minutes":         {"%s min", "%s মিনিট"},
	"optional":        {"optional", "ঐচ্ছিক"},
	"empty":           {"Nothing is scheduled.", "কিছু নির্ধারিত নেই।"},
	"none":            {"—", "—"},
	"kind_theory":     {"", ""},
	"kind_practical":  {"practical", "ব্যবহারিক"},
	"kind_viva":       {"viva", "মৌখিক"},
	"kind_oral":       {"oral", "মৌখিক পরীক্ষা"},
	"paper_1":         {"1st paper", "১ম পত্র"},
	"paper_2":         {"2nd paper", "২য় পত্র"},
	"paper_3":         {"3rd paper", "৩য় পত্র"},
	"room_classroom":  {"Classroom", "শ্রেণিকক্ষ"},
	"room_lab":        {"Laboratory", "গবেষণাগার"},
	"room_hall":       {"Hall", "হলরুম"},
	"capacity":        {"seats %s", "আসন %s"},
	"generated_draft": {"Draft for staff, prepared %s. Not for students.", "শিক্ষক ও কর্মীদের জন্য খসড়া, প্রস্তুত %s। শিক্ষার্থীদের জন্য নয়।"},
}

func (l lang) r(key string) string {
	v := routineLabels[key]
	if l == "bn" {
		return v[1]
	}
	return v[0]
}

var bnWeekdays = []string{"রবিবার", "সোমবার", "মঙ্গলবার", "বুধবার", "বৃহস্পতিবার", "শুক্রবার", "শনিবার"}

// day formats a calendar date with its weekday.
func (l lang) day(iso string) (string, string) {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso, ""
	}
	if l == "bn" {
		return l.int(t.Day()) + " " + bnMonths[t.Month()-1] + " " + l.int(t.Year()), bnWeekdays[t.Weekday()]
	}
	return t.Format("2 January 2006"), t.Weekday().String()
}

func (l lang) clock(m int) string { return l.num(fmt.Sprintf("%02d:%02d", m/60, m%60)) }

// RoutinePaperName is a paper's name in a language ("English 2nd paper",
// "Physics practical").
func RoutinePaperName(language string, subject Name, kind string, no int, many bool) string {
	l := lang(language)
	parts := []string{l.pick(subject.EN, subject.BN)}
	if no > 1 || many {
		parts = append(parts, l.r(fmt.Sprintf("paper_%d", no)))
	}
	if k := l.r("kind_" + kind); k != "" {
		parts = append(parts, k)
	}
	return strings.Join(parts, " ")
}

type routineTable struct {
	Heading  string     `json:"heading"`
	Note     string     `json:"note"`
	Columns  []float64  `json:"columns"`
	Headers  []string   `json:"headers"`
	Rows     [][]string `json:"rows"`
	Shade    []bool     `json:"shade"` // first row of a new date
	Empty    string     `json:"empty"`
	Centered []int      `json:"centered"`
}

type routineData struct {
	Title       string            `json:"title"`
	Lang        string            `json:"lang"`
	Kind        string            `json:"kind"`
	Heading     string            `json:"heading"`
	Subtitle    string            `json:"subtitle"`
	School      schoolData        `json:"school"`
	Provisional bool              `json:"provisional"`
	Labels      map[string]string `json:"labels"`
	Pages       []routineTable    `json:"pages"`
	Footer      []string          `json:"footer"`
	Notes       []string          `json:"notes"`
	Landscape   bool              `json:"landscape"`
}

// manyPapers marks subjects with more than one numbered paper in a page,
// so their first paper is labelled "1st paper".
func manyPapers(rows []RoutineRow) map[string]bool {
	seen := map[string]int{}
	for _, r := range rows {
		for _, p := range r.Papers {
			if p.PaperNo > 1 {
				seen[p.Class+"|"+p.Code] = 2
			}
		}
	}
	out := map[string]bool{}
	for k, v := range seen {
		out[k] = v > 1
	}
	return out
}

func (l lang) routineData(in RoutineDoc, school schoolData) routineData {
	title := l.r("title")
	switch in.Kind {
	case "room":
		title = l.r("rooms_title")
	case "invigilation":
		title = l.r("duties_title")
	}
	sub := []string{in.Term}
	if in.Session != "" {
		sub = append(sub, l.num(in.Session))
	}
	sub = append(sub, fmt.Sprintf(l.r("version"), l.int(in.Version)))
	out := routineData{Title: title + " — " + in.Term, Lang: string(l), Kind: in.Kind, Heading: title, Subtitle: strings.Join(sub, " · "), School: school,
		Provisional: in.Status != "published", Labels: map[string]string{"page": l.r("page"), "provisional": l.r("draft")}, Landscape: in.Kind == "invigilation"}
	if in.Status == "superseded" {
		out.Labels["provisional"] = l.r("superseded")
	}
	for _, pg := range in.Pages {
		t := routineTable{Heading: l.pick(pg.Title.EN, pg.Title.BN), Note: l.pick(pg.Note.EN, pg.Note.BN), Empty: l.r("empty"), Rows: [][]string{}, Shade: []bool{}}
		many := manyPapers(pg.Rows)
		paper := func(p RoutinePaper) string {
			s := RoutinePaperName(string(l), p.Subject, p.Kind, p.PaperNo, many[p.Class+"|"+p.Code])
			if p.Optional {
				s += " (" + l.r("optional") + ")"
			}
			return s
		}
		names := func(xs []Name) string {
			out := []string{}
			for _, n := range xs {
				out = append(out, l.pick(n.EN, n.BN))
			}
			if len(out) == 0 {
				return l.r("none")
			}
			return strings.Join(out, "\n")
		}
		last := ""
		for _, r := range pg.Rows {
			date, weekday := l.day(r.Date)
			dateCell := date + "\n" + weekday
			if r.Date == last {
				dateCell = ""
			}
			t.Shade = append(t.Shade, r.Date != last)
			last = r.Date
			sitting := l.pick(r.Sitting.EN, r.Sitting.BN) + "\n" + l.clock(r.Start) + "–" + l.clock(r.End)
			switch in.Kind {
			case "class":
				for _, p := range r.Papers {
					rooms := []string{}
					for _, u := range p.Rooms {
						name := l.pick(u.Room.EN, u.Room.BN)
						if u.Section != "" && len(p.Rooms) > 1 {
							name = u.Section + ": " + name
						}
						rooms = append(rooms, name)
					}
					if len(rooms) == 0 {
						rooms = append(rooms, l.r("none"))
					}
					code := ""
					if p.Code != "" {
						code = "\n" + p.Code
					}
					t.Rows = append(t.Rows, []string{dateCell, sitting, paper(p) + code, fmt.Sprintf(l.r("minutes"), l.int(p.Duration)), strings.Join(rooms, "\n")})
				}
			case "room":
				lines := []string{}
				for _, p := range r.Papers {
					lines = append(lines, p.Class+" "+p.Sections+" · "+paper(p))
				}
				t.Rows = append(t.Rows, []string{dateCell, sitting, strings.Join(lines, "\n"), l.int(r.Students), names(r.Invigilators)})
			case "invigilation":
				lines := []string{}
				for _, p := range r.Papers {
					lines = append(lines, p.Class+" "+p.Sections+" · "+paper(p))
				}
				t.Rows = append(t.Rows, []string{dateCell, sitting, l.pick(r.Room.EN, r.Room.BN), names(r.Invigilators), strings.Join(lines, "\n")})
			}
		}
		switch in.Kind {
		case "class":
			t.Columns, t.Headers, t.Centered = []float64{1.25, 1.15, 2.1, 0.75, 1.4}, []string{l.r("date"), l.r("sitting"), l.r("subject"), l.r("duration"), l.r("rooms")}, []int{3}
		case "room":
			t.Columns, t.Headers, t.Centered = []float64{1.2, 1.1, 2.6, 0.7, 1.5}, []string{l.r("date"), l.r("sitting"), l.r("classes"), l.r("students"), l.r("invigilators")}, []int{3}
		default:
			t.Columns, t.Headers, t.Centered = []float64{1.1, 1.1, 1.0, 1.6, 2.8}, []string{l.r("date"), l.r("sitting"), l.r("room"), l.r("invigilators"), l.r("classes")}, []int{}
		}
		out.Pages = append(out.Pages, t)
	}
	if len(in.Holidays) > 0 {
		hs := []string{}
		for _, h := range in.Holidays {
			d, _ := l.day(h.Date)
			hs = append(hs, d+" ("+l.pick(h.Title.EN, h.Title.BN)+")")
		}
		out.Notes = append(out.Notes, fmt.Sprintf(l.r("holidays"), strings.Join(hs, "; ")))
	}
	if s := strings.TrimSpace(in.Instructions); s != "" {
		out.Notes = append(out.Notes, l.r("instructions")+": "+s)
	}
	if in.Reason != "" && in.Version > 1 {
		out.Notes = append(out.Notes, fmt.Sprintf(l.r("amended"), in.Reason))
	}
	if in.PublishedAt != "" && (in.Status == "published" || in.Status == "superseded") {
		out.Footer = append(out.Footer, fmt.Sprintf(l.r("published"), l.date(in.PublishedAt)))
	} else {
		out.Footer = append(out.Footer, fmt.Sprintf(l.r("generated_draft"), l.date(time.Now().UTC().Format(time.RFC3339))))
	}
	out.Footer = append(out.Footer, fmt.Sprintf(l.r("version"), l.int(in.Version)))
	return out
}

// RoomNote describes a room on its page ("Laboratory · seats 40").
func RoomNote(kind string, capacity *int) Name {
	out := Name{}
	for _, l := range []lang{"en", "bn"} {
		s := l.r("room_" + kind)
		if capacity != nil {
			s += " · " + fmt.Sprintf(l.r("capacity"), l.int(*capacity))
		}
		if l == "en" {
			out.EN = s
		} else {
			out.BN = s
		}
	}
	return out
}

// RenderRoutine renders a routine document as a PDF.
func (s *Service) RenderRoutine(ctx context.Context, in RoutineDoc) ([]byte, error) {
	if s == nil || s.renderer == nil {
		return nil, ErrUnavailable
	}
	if in.Language != "bn" {
		in.Language = "en"
	}
	l := lang(in.Language)
	brand, err := s.branding(ctx, l)
	if err != nil {
		return nil, err
	}
	assets := map[string][]byte{}
	brand.assets(assets)
	created := time.Now()
	if t, err := time.Parse(time.RFC3339, in.PublishedAt); err == nil && in.Status == "published" {
		created = t
	}
	return s.renderer.Render(ctx, Render{Template: "routine.typ", Data: l.routineData(in, brand.school), Assets: assets, Created: created})
}

// RoutineAvailable reports whether routine documents can be rendered.
func (s *Service) RoutineAvailable() bool { return s != nil && s.renderer != nil }
