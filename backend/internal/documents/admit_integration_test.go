package documents

import (
	"strings"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
)

// TestPostgresAdmitCardData checks how admit cards are worded: Bangla
// digits and dates, the room and seat on the card when they are the same
// for every paper (else "see the timetable"), the school's rules or the
// built-in ones, the head teacher's name, photos and QR codes as assets.
func TestPostgresAdmitCardData(t *testing.T) {
	ctx, db, done := openDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	if err := tx.Exec("UPDATE school_settings SET head_teacher_name_en = 'Head Master', head_teacher_name_bn = 'প্রধান শিক্ষক' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	store, err := files.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeRenderer{}
	svc := NewService(tx, nil, store, fake, nil)
	svc.BaseURL = "https://school.example.test"
	roll := 7
	hall := Name{EN: "Hall", BN: "হলরুম"}
	paper := func(date string, start int, en, bn string, room Name, seat int) AdmitPaper {
		return AdmitPaper{Date: date, Start: start, End: start + 180, Subject: Name{EN: en, BN: bn}, Code: "101", Kind: "theory", PaperNo: 1, Room: room, Seat: seat}
	}
	many := []AdmitPaper{}
	for i, s := range []struct{ en, bn string }{{"Bangla", "বাংলা"}, {"English", "ইংরেজি"}, {"Mathematics", "গণিত"}, {"Science", "বিজ্ঞান"}, {"Bangladesh and Global Studies", "বাংলাদেশ ও বিশ্বপরিচয়"},
		{"Religion", "ধর্ম"}, {"ICT", "তথ্য ও যোগাযোগ প্রযুক্তি"}, {"Agriculture", "কৃষিশিক্ষা"}, {"Physical Education", "শারীরিক শিক্ষা"}, {"Arts and Crafts", "চারু ও কারুকলা"}, {"Career Education", "ক্যারিয়ার শিক্ষা"}} {
		p := paper("2190-06-"+[]string{"01", "02", "04", "05", "06", "08", "09", "11", "12", "13", "15"}[i], 600, s.en, s.bn, hall, 12)
		if i == 7 {
			p.Optional = true
		}
		many = append(many, p)
	}
	in := AdmitDoc{Language: "bn", Term: "Annual Examination", Session: "2190", Version: 2, PublishedAt: "2190-05-20T04:00:00Z", Cards: []AdmitCard{
		{AdmissionNo: "2190-0012", Name: Name{EN: "Rafiq Islam", BN: "রফিক ইসলাম"}, Class: "Nine", Section: "A", Roll: &roll, Token: "AbCdEfGhIjKlMnOpQrStUv", Papers: many},
		{AdmissionNo: "2190-0013", Name: Name{EN: "Sumaiya Akter"}, Class: "Nine", Section: "A", Token: "BbCdEfGhIjKlMnOpQrStUv", Papers: []AdmitPaper{
			paper("2190-06-01", 600, "Bangla", "বাংলা", hall, 3), paper("2190-06-02", 840, "Physics", "পদার্থবিজ্ঞান", Name{EN: "Lab"}, 1)}},
		{AdmissionNo: "2190-0014", Name: Name{EN: "No Papers"}, Class: "Nine", Section: "B"},
	}}
	if _, err := svc.RenderAdmit(ctx, in); err != nil {
		t.Fatal(err)
	}
	r := fake.last()
	d := r.Data.(admitData)
	if r.Template != "admit.typ" || d.Heading != "প্রবেশপত্র" || d.HeadingAlt != "Admit Card" || d.HeadName != "প্রধান শিক্ষক" || len(d.Cards) != 3 || len(d.Rules) != 6 {
		t.Fatalf("admit data: %+v", d)
	}
	first := d.Cards[0]
	if first.Name != "রফিক ইসলাম" || first.NameAlt != "Rafiq Islam" || first.Roll != "৭" || first.Room != "হলরুম" || first.Seat != "১২" || first.QR == "" || len(first.Rows) != 11 {
		t.Fatalf("first card: %+v", first)
	}
	if row := first.Rows[0]; row.Date != "১ জুন ২১৯০" || row.Time != "১০:০০–১৩:০০" || !strings.Contains(row.Subject, "বাংলা") || row.Seat != "১২" {
		t.Fatalf("row: %+v", row)
	}
	if !strings.Contains(first.Rows[7].Subject, "ঐচ্ছিক") {
		t.Fatalf("optional paper: %+v", first.Rows[7])
	}
	if _, ok := r.Assets[first.QR]; !ok {
		t.Fatal("no QR asset")
	}
	second := d.Cards[1]
	if second.Room != "সময়সূচি দেখুন" || second.Seat != "সময়সূচি দেখুন" || second.Name != "Sumaiya Akter" || second.NameAlt != "" || second.Roll != "—" {
		t.Fatalf("second card: %+v", second)
	}
	if d.Cards[2].Room != "—" || len(d.Cards[2].Rows) != 0 {
		t.Fatalf("a card without papers: %+v", d.Cards[2])
	}

	// The school's own rules replace the built-in ones.
	in.Language, in.Rules = "en", Name{EN: "Come early.\n\nBring a pen."}
	if _, err := svc.RenderAdmit(ctx, in); err != nil {
		t.Fatal(err)
	}
	if d := fake.last().Data.(admitData); len(d.Rules) != 2 || d.Rules[1] != "Bring a pen." || d.Cards[0].Rows[0].Date != "1 June 2190" {
		t.Fatalf("own rules: %+v", d.Rules)
	}
}
