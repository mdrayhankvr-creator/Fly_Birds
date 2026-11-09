package documents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Admit cards (প্রবেশপত্র): two per A4 sheet with a cut line. The routine
// package gathers each student's own papers, rooms and seats from a term's
// published routine; this file words and lays them out.

// AdmitDoc is a set of admit cards of one term.
type AdmitDoc struct {
	Language    string
	Term        string
	Session     string
	Version     int
	PublishedAt string // RFC 3339
	Rules       Name   // the school's rules; empty uses the built-in ones
	Cards       []AdmitCard
}

// AdmitCard is one student's card.
type AdmitCard struct {
	AdmissionNo string
	Name        Name
	Class       string
	Section     string
	Roll        *int
	Photo       []byte // JPEG or PNG, or nil
	Token       string // verification code for the QR ("" leaves it out)
	Papers      []AdmitPaper
}

// AdmitPaper is one paper on a card, with the student's room and seat.
type AdmitPaper struct {
	Date       string // YYYY-MM-DD
	Start, End int    // minutes after midnight
	Subject    Name
	Code       string
	Kind       string
	PaperNo    int
	Many       bool // the subject has more than one paper: name it
	Optional   bool
	Room       Name
	Seat       int // 0: none
}

type admitRow struct {
	Date    string `json:"date"`
	Day     string `json:"day"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
	Room    string `json:"room"`
	Seat    string `json:"seat"`
}

type admitCardData struct {
	Name      string     `json:"name"`
	NameAlt   string     `json:"name_alt"`
	StudentID string     `json:"student_id"`
	Class     string     `json:"class"`
	Section   string     `json:"section"`
	Roll      string     `json:"roll"`
	Room      string     `json:"room"` // when every paper is in one room
	Seat      string     `json:"seat"` // when the seat is the same throughout
	Photo     string     `json:"photo"`
	QR        string     `json:"qr"`
	Code      string     `json:"code"`
	Rows      []admitRow `json:"rows"`
}

type admitData struct {
	Lang        string            `json:"lang"`
	Provisional bool              `json:"provisional"` // always false (common.typ reads it)
	Title       string            `json:"title"`
	Heading     string            `json:"heading"`
	HeadingAlt  string            `json:"heading_alt"`
	Exam        string            `json:"exam"`
	Labels      map[string]string `json:"labels"`
	School      schoolData        `json:"school"`
	HeadName    string            `json:"head_name"`
	Rules       []string          `json:"rules"`
	Footer      string            `json:"footer"`
	Cards       []admitCardData   `json:"cards"`
}

var admitLabels = map[string][2]string{
	"heading":    {"Admit Card", "প্রবেশপত্র"},
	"name":       {"Name", "নাম"},
	"student_id": {"Student ID", "শিক্ষার্থী আইডি"},
	"class":      {"Class", "শ্রেণি"},
	"section":    {"Section", "শাখা"},
	"roll":       {"Roll", "রোল"},
	"room":       {"Room", "কক্ষ"},
	"seat":       {"Seat", "আসন"},
	"session":    {"Session", "শিক্ষাবর্ষ"},
	"date":       {"Date", "তারিখ"},
	"day":        {"Day", "বার"},
	"time":       {"Time", "সময়"},
	"subject":    {"Subject", "বিষয়"},
	"photo":      {"Photo", "ছবি"},
	"rules":      {"Rules for candidates", "পরীক্ষার্থীদের জন্য নিয়মাবলি"},
	"timetable":  {"Examination timetable", "পরীক্ষার সময়সূচি"},
	"see_table":  {"See the timetable", "সময়সূচি দেখুন"},
	"scan":       {"Scan to verify", "যাচাই করতে স্ক্যান করুন"},
	"head":       {"Head teacher", "প্রধান শিক্ষক"},
	"cut":        {"Cut here", "এখানে কাটুন"},
	"footer":     {"Routine version %s, published %s. Code: %s", "রুটিন সংস্করণ %s, প্রকাশ %s। কোড: %s"},
	"footer_nc":  {"Routine version %s, published %s.", "রুটিন সংস্করণ %s, প্রকাশ %s।"},
	"no_papers":  {"No paper is scheduled for this student.", "এই শিক্ষার্থীর কোনো পরীক্ষা নির্ধারিত নেই।"},
}

// DefaultAdmitRules are printed when the school has not written its own.
var DefaultAdmitRules = Name{
	EN: "Bring this admit card to every examination and show it when asked.\n" +
		"Arrive at the examination room at least 30 minutes before the paper starts.\n" +
		"Sit only in the room and seat given on this card.\n" +
		"Mobile phones, smart watches, notes and bags are not allowed in the room.\n" +
		"Do not talk, copy or exchange anything during the examination.\n" +
		"Breaking a rule may cancel the examination.",
	BN: "প্রতিটি পরীক্ষায় এই প্রবেশপত্র সঙ্গে আনতে হবে এবং চাইলে দেখাতে হবে।\n" +
		"পরীক্ষা শুরুর অন্তত ৩০ মিনিট আগে পরীক্ষা কক্ষে উপস্থিত হতে হবে।\n" +
		"এই প্রবেশপত্রে দেওয়া কক্ষ ও আসনেই বসতে হবে।\n" +
		"মোবাইল ফোন, স্মার্টওয়াচ, নোট ও ব্যাগ নিয়ে কক্ষে প্রবেশ নিষেধ।\n" +
		"পরীক্ষার সময় কথা বলা, নকল করা বা কিছু আদান-প্রদান করা যাবে না।\n" +
		"নিয়ম ভাঙলে পরীক্ষা বাতিল হতে পারে।",
}

func (l lang) a(key string) string {
	v := admitLabels[key]
	if l == "bn" {
		return v[1]
	}
	return v[0]
}

// admitData words a set of cards; photos and QR codes go into assets.
func (s *Service) admitData(ctx context.Context, in AdmitDoc, brand branding, assets map[string][]byte) (admitData, error) {
	l := lang(in.Language)
	d := admitData{Lang: string(l), Heading: l.a("heading"), Exam: in.Term + " · " + l.a("session") + " " + l.num(in.Session), School: brand.school, Labels: map[string]string{}, Cards: []admitCardData{}}
	d.HeadingAlt = lang("en").a("heading")
	if l == "en" {
		d.HeadingAlt = lang("bn").a("heading")
	}
	d.Title = d.Heading + " — " + in.Term
	for k := range admitLabels {
		d.Labels[k] = l.a(k)
	}
	var head struct{ EN, BN string }
	if err := s.db.WithContext(ctx).Raw("SELECT head_teacher_name_en AS en, head_teacher_name_bn AS bn FROM school_settings WHERE id = 1").Scan(&head).Error; err != nil {
		return d, err
	}
	d.HeadName = l.pick(head.EN, head.BN)
	if brand.school.Signatory != "" {
		d.Labels["head"] = brand.school.Signatory
	}
	rules := l.pick(in.Rules.EN, in.Rules.BN)
	if strings.TrimSpace(rules) == "" {
		rules = l.pick(DefaultAdmitRules.EN, DefaultAdmitRules.BN)
	}
	for _, r := range strings.Split(rules, "\n") {
		if r = strings.TrimSpace(r); r != "" {
			d.Rules = append(d.Rules, l.num(r))
		}
	}
	published := "—"
	if in.PublishedAt != "" {
		published = l.date(in.PublishedAt)
	}
	for i, c := range in.Cards {
		card := admitCardData{Name: l.pick(c.Name.EN, c.Name.BN), NameAlt: l.other(c.Name.EN, c.Name.BN), StudentID: l.num(c.AdmissionNo), Class: c.Class, Section: c.Section, Roll: "—", Rows: []admitRow{}}
		if c.Roll != nil {
			card.Roll = l.int(*c.Roll)
		}
		if len(c.Photo) > 0 {
			ext := map[string]string{"image/jpeg": "jpg", "image/png": "png"}[http.DetectContentType(c.Photo)]
			if ext != "" {
				card.Photo = fmt.Sprintf("aphoto-%d.%s", i, ext)
				assets[card.Photo] = c.Photo
			}
		}
		if c.Token != "" {
			card.Code = documentCode(c.Token)
			if s.BaseURL != "" {
				png, err := qrPNG(strings.TrimRight(s.BaseURL, "/") + "/verify/admit/" + c.Token)
				if err != nil {
					return d, err
				}
				card.QR = fmt.Sprintf("aqr-%d.png", i)
				assets[card.QR] = png
			}
		}
		rooms, seats := map[string]bool{}, map[int]bool{}
		for _, p := range c.Papers {
			date, day := l.day(p.Date)
			name := RoutinePaperName(string(l), p.Subject, p.Kind, p.PaperNo, p.Many)
			if p.Optional {
				name += " (" + l.r("optional") + ")"
			}
			if p.Code != "" {
				name = l.num(p.Code) + " · " + name
			}
			row := admitRow{Date: date, Day: day, Time: l.clock(p.Start) + "–" + l.clock(p.End), Subject: name, Room: l.pick(p.Room.EN, p.Room.BN), Seat: "—"}
			if row.Room == "" {
				row.Room = "—"
			}
			if p.Seat > 0 {
				row.Seat = l.int(p.Seat)
			}
			rooms[row.Room], seats[p.Seat] = true, true
			card.Rows = append(card.Rows, row)
		}
		card.Room, card.Seat = l.a("see_table"), l.a("see_table")
		if len(rooms) == 1 {
			for r := range rooms {
				card.Room = r
			}
		}
		if len(seats) == 1 {
			for st := range seats {
				card.Seat = "—"
				if st > 0 {
					card.Seat = l.int(st)
				}
			}
		}
		if len(c.Papers) == 0 {
			card.Room, card.Seat = "—", "—"
		}
		d.Cards = append(d.Cards, card)
	}
	if len(in.Cards) > 0 {
		d.Footer = fmt.Sprintf(l.a("footer_nc"), l.int(in.Version), published)
	}
	return d, nil
}

// RenderAdmit renders admit cards as a PDF, two per A4 page.
func (s *Service) RenderAdmit(ctx context.Context, in AdmitDoc) ([]byte, error) {
	if s == nil || s.renderer == nil {
		return nil, ErrUnavailable
	}
	if in.Language != "bn" {
		in.Language = "en"
	}
	brand, err := s.branding(ctx, lang(in.Language))
	if err != nil {
		return nil, err
	}
	assets := map[string][]byte{}
	brand.assets(assets)
	data, err := s.admitData(ctx, in, brand, assets)
	if err != nil {
		return nil, err
	}
	created := s.now()
	if t, err := time.Parse(time.RFC3339, in.PublishedAt); err == nil {
		created = t
	}
	return s.renderer.Render(ctx, Render{Template: "admit.typ", Data: data, Assets: assets, Created: created})
}
