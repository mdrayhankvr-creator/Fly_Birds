// Package schoolsetup contains the read-only first milestone of school setup.
// It has no apply operation. Every database query runs in one PostgreSQL
// REPEATABLE READ, READ ONLY transaction.
package schoolsetup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	StudentsPerSection = 15
	TeacherTarget      = 15
	LunchMinutes       = 45
)

// These two existing administrator accounts are never candidates for setup.
// A future apply milestone must enforce the same protection independently.
var protectedAdminEmails = [...]string{
	"2023331031@student.sust.edu",
	"mdrayhankvr@gmail.com",
}

// Manifest contains identifiers to check, not records to import. Deliberately
// no password, role, profile association, or school-setting field is accepted.
type Manifest struct {
	StudentAdmissionNumbers []string `json:"student_admission_numbers"`
	TeacherEmployeeCodes    []string `json:"teacher_employee_codes"`
	AccountEmails           []string `json:"account_emails"`
}

type Section struct {
	Class    string `json:"class"`
	Name     string `json:"section"`
	Target   int    `json:"target"`
	Existing int    `json:"existing"`
	ToAdd    int    `json:"to_add"`
}

type Teacher struct {
	EmployeeCode        string `json:"employee_code"`
	Name                string `json:"name"`
	Active              bool   `json:"active"`
	LinkedAccount       bool   `json:"linked_account"`
	LinkedRole          string `json:"linked_role,omitempty"`
	LinkedAccountActive bool   `json:"linked_account_active"`
	HasPhoto            bool   `json:"has_photo"`
	Assignments         int    `json:"assignments"`
}

type Subject struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type FourthSubject struct {
	Class                      string    `json:"class"`
	UsesGroups                 bool      `json:"uses_groups"`
	Options                    []Subject `json:"options"`
	ExistingWithoutChoice      int       `json:"existing_without_choice"`
	ProposedWithoutChoice      int       `json:"proposed_without_choice"`
	ExistingWithoutGroupChoice int       `json:"existing_without_group_choice,omitempty"`
	ProposedWithoutGroupChoice int       `json:"proposed_without_group_choice,omitempty"`
}

type AdminProtection struct {
	Email                       string `json:"email"`
	Present                     bool   `json:"present"`
	Role                        string `json:"role,omitempty"`
	Active                      bool   `json:"active"`
	ExcludedFromProposedChanges bool   `json:"excluded_from_proposed_changes"`
}

type IdentifierConflict struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type Report struct {
	ReadOnly             bool                 `json:"read_only"`
	School               string               `json:"school"`
	Session              string               `json:"session"`
	StudentsPerSection   int                  `json:"students_per_section"`
	StudentExisting      int                  `json:"student_existing"`
	StudentToAdd         int                  `json:"student_to_add"`
	Sections             []Section            `json:"sections"`
	TeacherTarget        int                  `json:"teacher_target"`
	TeacherExisting      int                  `json:"teacher_existing"`
	TeacherToAdd         int                  `json:"teacher_to_add"`
	Teachers             []Teacher            `json:"teachers"`
	ActiveSubjects       int                  `json:"active_subjects"`
	ClassSubjectMappings int                  `json:"class_subject_mappings"`
	FourthSubjects       []FourthSubject      `json:"fourth_subjects"`
	WeeklyOffDays        []int                `json:"weekly_off_days"`
	LunchMinutes         int                  `json:"lunch_minutes"`
	MissingRoutineInputs []string             `json:"missing_routine_inputs"`
	ManifestChecked      bool                 `json:"manifest_checked"`
	IdentifierConflicts  []IdentifierConflict `json:"identifier_conflicts"`
	ProtectedAdmins      []AdminProtection    `json:"protected_admins"`
	CredentialsGenerated bool                 `json:"credentials_generated"`
	Warnings             []string             `json:"warnings"`
}

type classRow struct {
	id, name   string
	usesGroups bool
	toAdd      int
}

func beginReadOnly(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

// Preview reads one consistent snapshot. There is intentionally no mutation
// method in this package, and PostgreSQL rejects writes in its transaction.
func Preview(ctx context.Context, db *sql.DB, manifest *Manifest) (Report, error) {
	r := Report{
		ReadOnly: true, StudentsPerSection: StudentsPerSection, TeacherTarget: TeacherTarget,
		LunchMinutes: LunchMinutes, Sections: []Section{}, Teachers: []Teacher{},
		FourthSubjects: []FourthSubject{}, WeeklyOffDays: []int{},
		IdentifierConflicts: []IdentifierConflict{}, ProtectedAdmins: []AdminProtection{}, Warnings: []string{},
		MissingRoutineInputs: []string{
			"school days and weekly off-day confirmation", "school opening and closing times", "period length and transitions",
			"45-minute lunch start time", "subject frequency by class and week",
			"teacher subject assignments, qualifications and availability", "room list and group split rules",
		},
	}
	tx, err := beginReadOnly(ctx, db)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT name_en FROM school_settings WHERE id=1").Scan(&r.School); err != nil {
		return Report{}, fmt.Errorf("school identity: %w", err)
	}
	var sessionID string
	if err := tx.QueryRowContext(ctx, "SELECT id::text, name FROM academic_sessions WHERE is_active").Scan(&sessionID, &r.Session); err != nil {
		return Report{}, fmt.Errorf("active session: %w", err)
	}

	classes := map[string]*classRow{}
	sectionRows, err := tx.QueryContext(ctx, `SELECT sec.class_id::text,c.name,c.uses_groups,sec.name,count(st.id)
FROM academic_sections sec JOIN academic_classes c ON c.id=sec.class_id
LEFT JOIN students st ON st.section_id=sec.id AND st.session_id=sec.session_id
WHERE sec.session_id=$1 AND sec.is_active AND c.is_active
GROUP BY sec.id,c.id ORDER BY c.display_order,c.name,sec.display_order,sec.name`, sessionID)
	if err != nil {
		return Report{}, fmt.Errorf("sections: %w", err)
	}
	for sectionRows.Next() {
		var id, className, sectionName string
		var usesGroups bool
		var existing int
		if err := sectionRows.Scan(&id, &className, &usesGroups, &sectionName, &existing); err != nil {
			sectionRows.Close()
			return Report{}, err
		}
		add := max(0, StudentsPerSection-existing)
		r.Sections = append(r.Sections, Section{Class: className, Name: sectionName, Target: StudentsPerSection, Existing: existing, ToAdd: add})
		r.StudentExisting += existing
		r.StudentToAdd += add
		if existing > StudentsPerSection {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s %s has %d students, above target %d", className, sectionName, existing, StudentsPerSection))
		}
		if classes[id] == nil {
			classes[id] = &classRow{id: id, name: className, usesGroups: usesGroups}
		}
		classes[id].toAdd += add
	}
	if err := sectionRows.Err(); err != nil {
		sectionRows.Close()
		return Report{}, err
	}
	sectionRows.Close()
	if len(r.Sections) == 0 {
		r.Warnings = append(r.Warnings, "no active sections in the active session")
	}

	teacherRows, err := tx.QueryContext(ctx, `SELECT e.employee_code,e.name_en,e.is_active,e.user_id IS NOT NULL,
coalesce(u.role,''),coalesce(u.is_active,false),p.employee_id IS NOT NULL,
(SELECT count(*) FROM teacher_assignments a WHERE a.employee_id=e.id AND a.session_id=$1)
FROM employees e LEFT JOIN users u ON u.id=e.user_id LEFT JOIN employee_photos p ON p.employee_id=e.id
WHERE e.kind='teacher' ORDER BY e.employee_code,e.id`, sessionID)
	if err != nil {
		return Report{}, fmt.Errorf("teachers: %w", err)
	}
	for teacherRows.Next() {
		var t Teacher
		if err := teacherRows.Scan(&t.EmployeeCode, &t.Name, &t.Active, &t.LinkedAccount, &t.LinkedRole, &t.LinkedAccountActive, &t.HasPhoto, &t.Assignments); err != nil {
			teacherRows.Close()
			return Report{}, err
		}
		r.Teachers = append(r.Teachers, t)
		if !t.LinkedAccount {
			r.Warnings = append(r.Warnings, t.EmployeeCode+" has no linked account; preview will not relink it")
		}
	}
	if err := teacherRows.Err(); err != nil {
		teacherRows.Close()
		return Report{}, err
	}
	teacherRows.Close()
	r.TeacherExisting = len(r.Teachers)
	r.TeacherToAdd = max(0, TeacherTarget-r.TeacherExisting)
	if r.TeacherExisting > TeacherTarget {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d teacher profiles exceed target %d", r.TeacherExisting, TeacherTarget))
	}
	if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM academic_subjects WHERE is_active),(SELECT count(*) FROM academic_class_subjects)").Scan(&r.ActiveSubjects, &r.ClassSubjectMappings); err != nil {
		return Report{}, err
	}

	optionRows, err := tx.QueryContext(ctx, `SELECT acs.class_id::text,s.code,s.name_en FROM academic_class_subjects acs
JOIN academic_subjects s ON s.id=acs.subject_id WHERE acs.is_fourth_subject ORDER BY s.code`)
	if err != nil {
		return Report{}, fmt.Errorf("fourth subjects: %w", err)
	}
	options := map[string][]Subject{}
	for optionRows.Next() {
		var id string
		var s Subject
		if err := optionRows.Scan(&id, &s.Code, &s.Name); err != nil {
			optionRows.Close()
			return Report{}, err
		}
		options[id] = append(options[id], s)
	}
	if err := optionRows.Err(); err != nil {
		optionRows.Close()
		return Report{}, err
	}
	optionRows.Close()
	// Preserve section/class order in the public report.
	seenClass := map[string]bool{}
	for _, section := range r.Sections {
		for _, c := range classes {
			if c.name != section.Class || seenClass[c.id] {
				continue
			}
			seenClass[c.id] = true
			f := FourthSubject{Class: c.name, UsesGroups: c.usesGroups, Options: append([]Subject{}, options[c.id]...), ProposedWithoutChoice: c.toAdd}
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM students st LEFT JOIN student_subject_choices ch
ON ch.student_id=st.id AND ch.session_id=st.session_id AND ch.role='optional'
WHERE st.session_id=$1 AND st.class_id=$2 AND ch.student_id IS NULL`, sessionID, c.id).Scan(&f.ExistingWithoutChoice); err != nil {
				return Report{}, err
			}
			if c.usesGroups {
				f.ProposedWithoutGroupChoice = c.toAdd
				if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM students st LEFT JOIN student_group_choices gc
ON gc.student_id=st.id AND gc.session_id=st.session_id WHERE st.session_id=$1 AND st.class_id=$2 AND gc.student_id IS NULL`, sessionID, c.id).Scan(&f.ExistingWithoutGroupChoice); err != nil {
					return Report{}, err
				}
			}
			r.FourthSubjects = append(r.FourthSubjects, f)
			hasHome, hasAgriculture := false, false
			for _, s := range f.Options {
				if s.Code == "151" {
					hasHome = true
				}
				if s.Code == "134" {
					hasAgriculture = true
				}
				if s.Code == "111" {
					r.Warnings = append(r.Warnings, c.name+" has Religion marked optional; school policy required")
				}
			}
			if !hasHome || !hasAgriculture {
				r.Warnings = append(r.Warnings, c.name+" lacks Home Science and/or Agriculture optional mapping; defaults cannot be previewed")
			}
		}
	}
	r.Warnings = append(r.Warnings, "gender-based fourth-subject defaults are unapproved; no choice is inferred from name or unknown gender")
	for _, c := range r.FourthSubjects {
		if c.UsesGroups {
			r.Warnings = append(r.Warnings, c.Class+" group choices require school-approved allocations")
		}
	}

	offRows, err := tx.QueryContext(ctx, "SELECT day_of_week FROM school_weekly_off_days ORDER BY day_of_week")
	if err != nil {
		return Report{}, err
	}
	for offRows.Next() {
		var day int
		if err := offRows.Scan(&day); err != nil {
			offRows.Close()
			return Report{}, err
		}
		r.WeeklyOffDays = append(r.WeeklyOffDays, day)
	}
	if err := offRows.Err(); err != nil {
		offRows.Close()
		return Report{}, err
	}
	offRows.Close()
	for _, email := range protectedAdminEmails {
		a := AdminProtection{Email: email, ExcludedFromProposedChanges: true}
		err := tx.QueryRowContext(ctx, "SELECT role,is_active FROM users WHERE lower(email)=$1", email).Scan(&a.Role, &a.Active)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Report{}, err
		}
		a.Present = err == nil
		if !a.Present || a.Role != "admin" || !a.Active {
			r.Warnings = append(r.Warnings, "protected administrator "+email+" is missing, inactive, or not an admin")
		}
		r.ProtectedAdmins = append(r.ProtectedAdmins, a)
	}
	if err := checkIdentifiers(ctx, tx, manifest, &r); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return r, nil
}

func checkIdentifiers(ctx context.Context, tx *sql.Tx, manifest *Manifest, r *Report) error {
	for _, spec := range []struct{ kind, query string }{
		{"admission_number", "SELECT lower(admission_no) FROM students GROUP BY lower(admission_no) HAVING count(*)>1"},
		{"employee_code", "SELECT upper(employee_code) FROM employees GROUP BY upper(employee_code) HAVING count(*)>1"},
		{"email", "SELECT lower(email) FROM users GROUP BY lower(email) HAVING count(*)>1"},
	} {
		rows, err := tx.QueryContext(ctx, spec.query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			r.IdentifierConflicts = append(r.IdentifierConflicts, IdentifierConflict{spec.kind, key, "duplicate_in_database"})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if manifest == nil {
		r.Warnings = append(r.Warnings, "candidate identifier conflicts not checked; provide --manifest with proposed identifiers")
		return nil
	}
	r.ManifestChecked = true
	for _, spec := range []struct {
		kind, query string
		values      []string
	}{
		{"admission_number", "SELECT lower(admission_no) FROM students", manifest.StudentAdmissionNumbers},
		{"employee_code", "SELECT upper(employee_code) FROM employees", manifest.TeacherEmployeeCodes},
		{"email", "SELECT lower(email) FROM users", manifest.AccountEmails},
	} {
		if len(spec.values) > 1000 {
			return fmt.Errorf("too many %s candidates", spec.kind)
		}
		existing := map[string]bool{}
		rows, err := tx.QueryContext(ctx, spec.query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			existing[key] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		seen := map[string]bool{}
		for _, raw := range spec.values {
			value := strings.TrimSpace(raw)
			key := strings.ToLower(value)
			if spec.kind == "employee_code" {
				key = strings.ToUpper(value)
			}
			if value == "" || len(value) > 254 {
				return fmt.Errorf("invalid %s candidate", spec.kind)
			}
			if seen[key] {
				r.IdentifierConflicts = append(r.IdentifierConflicts, IdentifierConflict{spec.kind, value, "duplicate_in_manifest"})
			}
			seen[key] = true
			if existing[key] {
				r.IdentifierConflicts = append(r.IdentifierConflicts, IdentifierConflict{spec.kind, value, "already_exists"})
			}
			if spec.kind == "email" {
				for _, admin := range protectedAdminEmails {
					if key == admin {
						r.IdentifierConflicts = append(r.IdentifierConflicts, IdentifierConflict{spec.kind, value, "protected_admin"})
					}
				}
			}
		}
	}
	return nil
}
