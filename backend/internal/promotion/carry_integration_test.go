package promotion

import (
	"errors"
	"testing"
)

// Promotion carries the group and the fourth subject into the next year:
// the batch prefills them from the closing year, they are editable while it
// is open (only values the target class can take), applying writes them as
// the new session's choices, and a revert removes them again.
func TestPostgresPromotionCarriesGroupAndFourth(t *testing.T) {
	ctx, db := openDB(t)
	tx := db.Begin()
	defer tx.Rollback()
	s := newSchool(t, tx, 2403)
	ps := NewService(tx)
	admin, head := Actor{s.admin, "admin"}, Actor{s.head, "head_teacher"}

	// Both classes use groups; physics belongs to Science in both; higher
	// math is the fourth subject of both; agriculture is a fourth subject of
	// class ten only.
	s.exec("UPDATE academic_classes SET uses_groups = true WHERE id IN (?, ?)", s.ten, s.eleven)
	science := s.id("SELECT id::text FROM academic_groups WHERE name_en = 'Science'")
	humanities := s.id("SELECT id::text FROM academic_groups WHERE name_en = 'Humanities'")
	subject := func(name, code string) string {
		return s.id("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", name+" "+s.stamp, name, code)
	}
	physics := subject("CF Physics", "CFP-"+s.stamp[9:])
	higherMath := subject("CF Higher Math", "CFH-"+s.stamp[9:])
	agriculture := subject("CF Agriculture", "CFA-"+s.stamp[9:])
	for _, class := range []string{s.ten, s.eleven} {
		s.exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", class, physics)
		s.exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", class, higherMath)
		s.exec("INSERT INTO class_group_subjects(class_id,group_id,subject_id) VALUES (?,?,?)", class, science, physics)
	}
	s.exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", s.ten, agriculture)

	carried, _ := s.student(1)
	s.exec("INSERT INTO student_group_choices(student_id,session_id,group_id) VALUES (?,?,?)", carried, s.from, science)
	s.exec("INSERT INTO student_subject_choices(student_id,session_id,subject_id,role) VALUES (?,?,?,'optional')", carried, s.from, higherMath)
	dropped, _ := s.student(2) // the fourth subject class eleven does not offer
	s.exec("INSERT INTO student_subject_choices(student_id,session_id,subject_id,role) VALUES (?,?,?,'optional')", dropped, s.from, agriculture)

	// Next year: class eleven with one section, so placement is automatic.
	s.exec("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A')", s.to, s.eleven)
	next := s.eleven
	if _, err := ps.SetProgression(ctx, admin, s.ten, ProgressionInput{NextClassID: &next}); err != nil {
		t.Fatal(err)
	}
	b, err := ps.Create(ctx, admin, CreateInput{FromSessionID: s.from, ToSessionID: s.to, ClassID: s.ten})
	if err != nil {
		t.Fatal(err)
	}
	// No published annual result: every student needs a decision with a reason.
	promote := Promote
	reason := "promoted by the teachers' meeting"
	for _, st := range []string{carried, dropped} {
		if b, err = ps.UpdateItem(ctx, admin, b.ID, st, ItemInput{Decision: &promote, Reason: &reason}); err != nil {
			t.Fatal(err)
		}
	}
	itemOf := func(b Batch, student string) Item {
		for _, it := range b.Items {
			if it.StudentID == student {
				return it
			}
		}
		t.Fatalf("student %s is not in the batch", student)
		return Item{}
	}
	one := itemOf(b, carried)
	if one.TargetGroupID == nil || *one.TargetGroupID != science || one.TargetFourthSubjectID == nil || *one.TargetFourthSubjectID != higherMath {
		t.Fatalf("the closing year's choices are not prefilled: %+v", one)
	}
	if !one.TargetClassUsesGroups || one.SourceGroupName == nil || one.SourceFourthSubjectName == nil {
		t.Fatalf("the item does not describe the carried values: %+v", one)
	}
	// A fourth subject the target class does not offer is dropped, not carried.
	two := itemOf(b, dropped)
	if two.TargetFourthSubjectID != nil {
		t.Fatalf("a fourth subject class eleven does not offer was carried: %+v", two)
	}

	// The batch can change the carried values, to ones the target class takes.
	bad := "00000000-0000-4000-8000-000000000001"
	if _, err := ps.UpdateItem(ctx, admin, b.ID, carried, ItemInput{TargetGroupID: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an unknown group: %v", err)
	}
	if _, err := ps.UpdateItem(ctx, admin, b.ID, carried, ItemInput{TargetFourthSubjectID: &agriculture}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a fourth subject the target class does not offer: %v", err)
	}
	if b, err = ps.UpdateItem(ctx, admin, b.ID, carried, ItemInput{TargetGroupID: &humanities}); err != nil {
		t.Fatal(err)
	}
	if one = itemOf(b, carried); *one.TargetGroupID != humanities {
		t.Fatalf("the edited group did not stick: %+v", one)
	}

	// Approve and apply: the choices land in the new session.
	if b, err = ps.Approve(ctx, head, b.ID, HashInput{InputsSHA256: b.InputsSHA256}); err != nil {
		t.Fatal(err)
	}
	if b, err = ps.Apply(ctx, admin, b.ID); err != nil {
		t.Fatal(err)
	}
	var group, fourth string
	if err := tx.Raw("SELECT group_id::text FROM student_group_choices WHERE student_id = ? AND session_id = ?", carried, s.to).Row().Scan(&group); err != nil || group != humanities {
		t.Fatalf("the applied group: %q %v", group, err)
	}
	if err := tx.Raw("SELECT subject_id::text FROM student_subject_choices WHERE student_id = ? AND session_id = ? AND role = 'optional'", carried, s.to).Row().Scan(&fourth); err != nil || fourth != higherMath {
		t.Fatalf("the applied fourth subject: %q %v", fourth, err)
	}
	var n int64
	if err := tx.Raw("SELECT count(*) FROM student_subject_choices WHERE student_id = ? AND session_id = ?", dropped, s.to).Row().Scan(&n); err != nil || n != 0 {
		t.Fatalf("the dropped fourth subject reappeared: %d %v", n, err)
	}

	// The choices the apply wrote do not block the revert, and go with it.
	if b, err = ps.Revert(ctx, admin, b.ID, RevertInput{Reason: "applied a year early"}); err != nil {
		t.Fatalf("revert: %v (blockers %+v)", err, b.RevertBlockers)
	}
	if err := tx.Raw("SELECT count(*) FROM student_group_choices WHERE session_id = ?", s.to).Row().Scan(&n); err != nil || n != 0 {
		t.Fatalf("group choices after the revert: %d %v", n, err)
	}
	if err := tx.Raw("SELECT count(*) FROM student_subject_choices WHERE session_id = ?", s.to).Row().Scan(&n); err != nil || n != 0 {
		t.Fatalf("subject choices after the revert: %d %v", n, err)
	}
	// The closing year's own choices are untouched.
	if err := tx.Raw("SELECT count(*) FROM student_group_choices WHERE student_id = ? AND session_id = ?", carried, s.from).Row().Scan(&n); err != nil || n != 1 {
		t.Fatalf("the old year's group choice: %d %v", n, err)
	}
}
