package blueprints

import (
	"errors"
	"fmt"
	"testing"
)

func TestPostgresFourthPolicyDefaultsAndScienceSwap(t *testing.T) {
	for _, grade := range []int{6, 7, 8, 9, 10} {
		t.Run(fmt.Sprint(grade), func(t *testing.T) {
			f, done := newFixture(t, 2300+grade)
			defer done()
			insert := func(q string, args ...any) string {
				t.Helper()
				var id string
				if err := f.tx.Raw(q, args...).Row().Scan(&id); err != nil {
					t.Fatal(err)
				}
				return id
			}
			exec := func(q string, args ...any) {
				t.Helper()
				if err := f.tx.Exec(q, args...).Error; err != nil {
					t.Fatal(err)
				}
			}
			takes := func(student, subject string) bool {
				t.Helper()
				var yes bool
				if err := f.tx.Raw("SELECT student_takes_subject(?,?,?,?)", student, f.session, f.class, f.subjects[subject]).Row().Scan(&yes); err != nil {
					t.Fatal(err)
				}
				return yes
			}
			fourth := func(student, subject string) bool {
				t.Helper()
				var yes bool
				if err := f.tx.Raw("SELECT student_subject_is_fourth(?,?,?,?)", student, f.session, f.class, f.subjects[subject]).Row().Scan(&yes); err != nil {
					t.Fatal(err)
				}
				return yes
			}
			exec("UPDATE academic_classes SET name=? WHERE id=?", fmt.Sprintf("CLASS-%d-%s", grade, f.stamp), f.class)
			f.subjects["S4"] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", "Home Science "+f.stamp, "HS"+f.stamp[len(f.stamp)-8:])
			exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", f.class, f.subjects["S4"])
			for i, gender := range []string{"male", "female", ""} {
				exec("UPDATE students SET gender=? WHERE id=?", gender, f.students[i])
			}
			policy := FourthPolicy{Enabled: true, MaleSubjectID: f.subjects["S3"], FemaleSubjectID: f.subjects["S4"]}
			if grade == 6 {
				religion := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES ('Religion','Bangla','111') RETURNING id")
				exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", f.class, religion)
				if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, policy); !errors.Is(err, ErrInvalid) {
					t.Fatalf("optional Religion accepted: %v", err)
				}
				exec("UPDATE academic_class_subjects SET is_required=true,is_fourth_subject=false WHERE class_id=? AND subject_id=?", f.class, religion)
				secondReligion := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", "Religion 2 "+f.stamp, "R2"+f.stamp[len(f.stamp)-8:])
				exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", f.class, secondReligion)
				if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, policy); !errors.Is(err, ErrInvalid) {
					t.Fatalf("multiple Religion courses accepted: %v", err)
				}
				if view, err := f.svc.Choices(f.ctx, f.session, f.class); err != nil || !hasWarning(view.Warnings, "multiple_religion_courses") {
					t.Fatalf("missing multiple Religion warning: %+v %v", view.Warnings, err)
				}
				exec("DELETE FROM academic_class_subjects WHERE class_id=? AND subject_id=?", f.class, secondReligion)
			}
			if grade >= 9 {
				exec("UPDATE academic_classes SET uses_groups=true WHERE id=?", f.class)
				group := insert("SELECT id FROM academic_groups WHERE name_en='Science'")
				for i, key := range []string{"S5", "S6"} {
					f.subjects[key] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", key+f.stamp, fmt.Sprintf("P%d%s", i, f.stamp[len(f.stamp)-8:]))
					exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", f.class, f.subjects[key])
					exec("INSERT INTO class_group_subjects(class_id,group_id,subject_id) VALUES (?,?,?)", f.class, group, f.subjects[key])
				}
				exec("INSERT INTO student_group_choices(student_id,session_id,group_id) VALUES (?,?,?)", f.students[0], f.session, group)
				policy.SwapGroupID, policy.SwapSubjectAID, policy.SwapSubjectBID = group, f.subjects["S5"], f.subjects["S6"]
				exec("INSERT INTO student_subject_choices(student_id,session_id,role,subject_id) VALUES (?,?,'optional',?)", f.students[1], f.session, f.subjects["S5"])
				if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, policy); !errors.Is(err, ErrInvalid) {
					t.Fatalf("policy accepted ineligible existing choice: %v", err)
				}
				if view, err := f.svc.Choices(f.ctx, f.session, f.class); err != nil || !hasWarning(view.Warnings, "ineligible_existing_choices") {
					t.Fatalf("missing ineligible existing choice warning: %+v %v", view.Warnings, err)
				}
				exec("DELETE FROM student_subject_choices WHERE student_id=? AND session_id=?", f.students[1], f.session)
				bad := policy
				bad.SwapSubjectAID = bad.MaleSubjectID
				if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, bad); !errors.Is(err, ErrInvalid) {
					t.Fatalf("gender option in science pair accepted: %v", err)
				}
			}
			if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, policy); err != nil {
				t.Fatal(err)
			}
			exec("SAVEPOINT policy_mapping_guard")
			if err := f.tx.Exec("UPDATE academic_subjects SET is_active=false WHERE id=?", policy.MaleSubjectID).Error; err == nil {
				t.Fatal("referenced fourth subject deactivated")
			}
			exec("ROLLBACK TO SAVEPOINT policy_mapping_guard")
			if grade >= 9 {
				exec("SAVEPOINT policy_mapping_guard")
				if err := f.tx.Exec("INSERT INTO class_group_subjects(class_id,group_id,subject_id) VALUES (?,?,?)", f.class, policy.SwapGroupID, policy.MaleSubjectID).Error; err == nil {
					t.Fatal("referenced gender subject group mapping changed")
				}
				exec("ROLLBACK TO SAVEPOINT policy_mapping_guard")
			}
			if !takes(f.students[0], "S3") || takes(f.students[0], "S4") || !fourth(f.students[0], "S3") ||
				!takes(f.students[1], "S4") || takes(f.students[1], "S3") || !fourth(f.students[1], "S4") ||
				takes(f.students[2], "S3") || takes(f.students[2], "S4") {
				t.Fatal("gender defaults or unknown-gender handling wrong")
			}
			view, err := f.svc.Choices(f.ctx, f.session, f.class)
			if err != nil || view.Unresolved != 1 || view.Students[2].ChoiceSource != "unresolved" {
				t.Fatalf("preview: %+v %v", view, err)
			}
			if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[2], SubjectID: f.subjects["S3"]}}); err != nil {
				t.Fatalf("explicit unknown-gender override: %v", err)
			}
			if grade >= 9 {
				f.subjects["S7"] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", "S7 "+f.stamp, "P7"+f.stamp[len(f.stamp)-8:])
				exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,false,true)", f.class, f.subjects["S7"])
				if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[2], SubjectID: f.subjects["S7"]}}); err != nil {
					t.Fatalf("eligible non-default choice: %v", err)
				}
				exec("SAVEPOINT policy_choice_mapping_guard")
				if err := f.tx.Exec("DELETE FROM academic_class_subjects WHERE class_id=? AND subject_id=?", f.class, f.subjects["S7"]).Error; err == nil {
					t.Fatal("mapping for explicit non-default choice removed")
				}
				exec("ROLLBACK TO SAVEPOINT policy_choice_mapping_guard")
				exec("SAVEPOINT policy_choice_mapping_guard")
				if err := f.tx.Exec("UPDATE academic_subjects SET is_active=false WHERE id=?", f.subjects["S7"]).Error; err == nil {
					t.Fatal("subject for explicit non-default choice deactivated")
				}
				exec("ROLLBACK TO SAVEPOINT policy_choice_mapping_guard")
				exec("SAVEPOINT policy_choice_mapping_guard")
				if err := f.tx.Exec("INSERT INTO class_group_subjects(class_id,group_id,subject_id) VALUES (?,?,?)", f.class, policy.SwapGroupID, f.subjects["S7"]).Error; err == nil {
					t.Fatal("group restriction added to explicit non-default choice")
				}
				exec("ROLLBACK TO SAVEPOINT policy_choice_mapping_guard")
				if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[1], SubjectID: f.subjects["S5"]}}); !errors.Is(err, ErrInvalid) {
					t.Fatalf("group-ineligible override accepted: %v", err)
				}
				if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: f.subjects["S5"]}}); err != nil {
					t.Fatal(err)
				}
				if !takes(f.students[0], "S5") || !takes(f.students[0], "S6") || !fourth(f.students[0], "S5") || fourth(f.students[0], "S6") || takes(f.students[0], "S3") || takes(f.students[0], "S4") {
					t.Fatal("science main/fourth swap or mutual exclusion wrong")
				}
				exec("SAVEPOINT policy_choice_group_guard")
				exec("DELETE FROM student_group_choices WHERE student_id=? AND session_id=?", f.students[0], f.session)
				if err := f.tx.Exec("SET CONSTRAINTS student_group_choices_fourth_policy_choice_guard IMMEDIATE").Error; err == nil {
					t.Fatal("group change made explicit fourth choice ineligible")
				}
				exec("ROLLBACK TO SAVEPOINT policy_choice_group_guard")
				if grade == 9 {
					publication := insert(`INSERT INTO result_publications(scope,scope_id,session_id,class_id,section_id,revision,kind,status,rules,rules_sha256,inputs_sha256,published_at)
VALUES ('annual',?,?,?, ?,1,'publication','published','{}','test',repeat('0',64),now()) RETURNING id`, f.session, f.session, f.class, f.sect)
					version := insert(`INSERT INTO result_versions(publication_id,scope,scope_id,session_id,class_id,section_id,student_id,version,status,outcome,total_marks,full_marks,gpa,inputs_sha256,snapshot,published_at)
VALUES (?,'annual',?,?,?,?,?,1,'published','passed',80,100,4,repeat('0',64),'{"format":"1","inputs":{},"outputs":{}}',now()) RETURNING id`, publication, f.session, f.session, f.class, f.sect, f.students[0])
					var originalSnapshot string
					if err := f.tx.Raw("SELECT snapshot::text FROM result_versions WHERE id=?", version).Row().Scan(&originalSnapshot); err != nil {
						t.Fatal(err)
					}
					locked := policy
					locked.Enabled = false
					if _, err := f.svc.SaveFourthPolicy(f.ctx, f.admin, f.class, locked); !errors.Is(err, ErrInUse) {
						t.Fatalf("published class policy changed: %v", err)
					}
					exec("SAVEPOINT gender_guard")
					if err := f.tx.Exec("UPDATE students SET gender='female' WHERE id=?", f.students[0]).Error; err == nil {
						t.Fatal("published result allowed gender change")
					}
					exec("ROLLBACK TO SAVEPOINT gender_guard")
					if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: f.subjects["S6"]}}); !errors.Is(err, ErrInUse) {
						t.Fatalf("published choice changed without review: %v", err)
					}
					if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: f.subjects["S6"]}}, true); err != nil {
						t.Fatalf("reviewed override: %v", err)
					}
					var stored string
					if err := f.tx.Raw("SELECT snapshot::text FROM result_versions WHERE id=?", version).Row().Scan(&stored); err != nil {
						t.Fatal(err)
					}
					if stored != originalSnapshot || !fourth(f.students[0], "S6") || fourth(f.students[0], "S5") {
						t.Fatal("published snapshot or reviewed fourth status wrong")
					}
				}
			}
		})
	}
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}
