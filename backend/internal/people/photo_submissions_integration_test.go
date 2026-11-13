package people

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func testPhoto(t *testing.T, w, h int, kind string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	var err error
	if kind == "image/png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestPostgresStudentPhotoSubmissions: a student's own photo waits for the
// class teacher or the administrator; only approval puts it into use, a
// rejection carries a reason the student sees, other teachers cannot review
// it, and the school can turn uploads off.
func TestPostgresStudentPhotoSubmissions(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	n := 0
	user := func(role string) string {
		n++
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("ph-%s%d-%s@example.test", role, n, unique), "unused", role, role+" "+unique)
	}
	admin, classTeacher, otherTeacher, stranger := user("admin"), user("teacher"), user("teacher"), user("student")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2192','2192-01-01','2192-12-31',false) RETURNING id")
	class := insert("INSERT INTO academic_classes(name,display_order) VALUES (?,7) RETURNING id", "Seven "+unique)
	sectionA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, class)
	sectionB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, class)
	pupil := func(i int, section string) (string, string) {
		u := user("student")
		return u, insert("INSERT INTO students(admission_no,name_en,roll_no,session_id,class_id,section_id,user_id,status) VALUES (?,?,?,?,?,?,?,'active') RETURNING id",
			fmt.Sprintf("PH-%s-%d", unique, i), fmt.Sprintf("Pupil %d", i), i, session, class, section, u)
	}
	aUser, aStudent := pupil(1, sectionA)
	bUser, bStudent := pupil(2, sectionB)
	emp := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Class Teacher','Teacher','2190-01-01') RETURNING id", "PH1"+unique[len(unique)-10:], classTeacher)
	insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Other','Teacher','2190-01-01') RETURNING id", "PH2"+unique[len(unique)-10:], otherTeacher)
	exec("INSERT INTO section_class_teachers(section_id,class_id,session_id,employee_id) VALUES (?,?,?,?)", sectionA, class, session, emp)
	exec("UPDATE school_settings SET student_photo_uploads = true WHERE id = 1")

	p := NewPhotoSubmissions(tx)
	studentA, studentB := Actor{ID: aUser, Role: "student"}, Actor{ID: bUser, Role: "student"}
	ct, other, adm := Actor{ID: classTeacher, Role: "teacher"}, Actor{ID: otherTeacher, Role: "teacher"}, Actor{ID: admin, Role: "admin"}
	inUse := func(student string) bool {
		var ok bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM student_photos WHERE student_id = ?)", student).Scan(&ok).Error; err != nil {
			t.Fatal(err)
		}
		return ok
	}

	// Nothing yet; uploads are on by default.
	if v, err := p.Mine(ctx, studentA); err != nil || !v.Enabled || v.Status != "" || v.HasPhoto || v.StudentID != aStudent {
		t.Fatalf("fresh state %+v %v", v, err)
	}
	// A user without a student record cannot upload.
	if _, err := p.Submit(ctx, Actor{ID: stranger, Role: "student"}, "image/jpeg", testPhoto(t, 300, 300, "image/jpeg")); !errors.Is(err, ErrNoStudent) {
		t.Fatalf("stranger upload: %v", err)
	}
	// Not an image, or too small: refused.
	if _, err := p.Submit(ctx, studentA, "image/jpeg", []byte("not a photo")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("garbage upload: %v", err)
	}
	if _, err := p.Submit(ctx, studentA, "image/png", testPhoto(t, 40, 40, "image/png")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tiny upload: %v", err)
	}

	// A wide PNG is cropped to a square JPEG and waits; it is not in use.
	v, err := p.Submit(ctx, studentA, "image/png", testPhoto(t, 900, 600, "image/png"))
	if err != nil || v.Status != "pending" || v.HasPhoto || inUse(aStudent) {
		t.Fatalf("submitted %+v %v", v, err)
	}
	mine, err := p.MyImage(ctx, studentA)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(mine)); err != nil || format != "jpeg" || cfg.Width != cfg.Height || cfg.Width > 480 {
		t.Fatalf("stored photo %v %+v %v", format, cfg, err)
	}

	// Review scope: the class teacher of section A and the administrator see
	// it; another teacher does not, and cannot open or approve it.
	list := func(a Actor) int {
		items, err := p.Pending(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	if list(ct) != 1 || list(adm) != 1 || list(other) != 0 {
		t.Fatalf("pending lists ct=%d admin=%d other=%d", list(ct), list(adm), list(other))
	}
	if _, err := p.ReviewImage(ctx, other, aStudent); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other teacher image: %v", err)
	}
	if err := p.Approve(ctx, other, aStudent); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other teacher approve: %v", err)
	}
	if inUse(aStudent) {
		t.Fatal("photo in use before approval")
	}

	// Rejection needs a reason, which the student then sees.
	if err := p.Reject(ctx, ct, aStudent, "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty reason: %v", err)
	}
	if err := p.Reject(ctx, ct, aStudent, "Please use a photo with a plain background."); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.Mine(ctx, studentA); v.Status != "rejected" || v.Reason != "Please use a photo with a plain background." || v.HasPhoto || v.ReviewedAt == nil {
		t.Fatalf("after rejection %+v", v)
	}
	if err := p.Approve(ctx, ct, aStudent); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approve a rejected photo: %v", err)
	}
	if list(ct) != 0 {
		t.Fatal("rejected photo still listed")
	}

	// A new upload replaces it and waits again; approval puts it into use and
	// keeps no other copy.
	if v, err := p.Submit(ctx, studentA, "image/jpeg", testPhoto(t, 500, 500, "image/jpeg")); err != nil || v.Status != "pending" || v.Reason != "" {
		t.Fatalf("resubmitted %+v %v", v, err)
	}
	if err := p.Approve(ctx, ct, aStudent); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.Mine(ctx, studentA); v.Status != "approved" || !v.HasPhoto || !inUse(aStudent) {
		t.Fatalf("after approval %+v", v)
	}
	if _, err := p.MyImage(ctx, studentA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approved submission kept its image: %v", err)
	}
	if err := p.Approve(ctx, ct, aStudent); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approve twice: %v", err)
	}

	// Section B has no class teacher: only the administrator reviews it.
	if _, err := p.Submit(ctx, studentB, "image/jpeg", testPhoto(t, 400, 400, "image/jpeg")); err != nil {
		t.Fatal(err)
	}
	if list(ct) != 0 || list(adm) != 1 {
		t.Fatal("section B photo listed for the wrong reviewer")
	}
	if err := p.Approve(ctx, adm, bStudent); err != nil || !inUse(bStudent) {
		t.Fatalf("admin approve: %v", err)
	}

	// Turned off: no new uploads, and the state says so.
	exec("UPDATE school_settings SET student_photo_uploads = false WHERE id = 1")
	if _, err := p.Submit(ctx, studentA, "image/jpeg", testPhoto(t, 300, 300, "image/jpeg")); !errors.Is(err, ErrPhotoUploadsOff) {
		t.Fatalf("upload while off: %v", err)
	}
	if v, _ := p.Mine(ctx, studentA); v.Enabled {
		t.Fatal("state says enabled while off")
	}

	// Every step is audited.
	var audited int64
	tx.Raw("SELECT count(*) FROM audit_events WHERE target_id IN (?, ?) AND action LIKE 'student.photo_%'", aStudent, bStudent).Scan(&audited)
	if audited != 6 {
		t.Fatalf("audit rows %d, want 6 (3 submitted, 1 rejected, 2 approved)", audited)
	}
}
