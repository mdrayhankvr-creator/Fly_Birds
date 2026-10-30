package people

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"gorm.io/gorm"
)

// One-page admission: the student, an optional student login and up to two
// guardians are created in one transaction, so a failure at any step leaves
// nothing behind (no half-created student, no orphan account or guardian).

var (
	ErrGuardianMatch    = errors.New("a guardian with this phone or email is already on record")
	ErrLoginTaken       = errors.New("login email already registered")
	ErrAdmissionNoTaken = errors.New("admission number already used")
	ErrRollTaken        = errors.New("roll number already used in this section")
	// ErrAdmissionKeyReused: an idempotency key already admitted a student
	// with a different admission number, so this is not a retry of it.
	ErrAdmissionKeyReused = errors.New("admission key already used for another admission")
	errAdmissionKeyTaken  = errors.New("admission key recorded concurrently")
	admissionNumberShape  = regexp.MustCompile(`^(.*?)(\d+)$`)
)

// GuardianMatchError reports existing guardians that look like the new one
// at Index, so staff can link the existing record instead of a duplicate.
type GuardianMatchError struct {
	Index   int             `json:"index"`
	Matches []GuardianMatch `json:"matches"`
}

func (e *GuardianMatchError) Error() string        { return ErrGuardianMatch.Error() }
func (e *GuardianMatchError) Is(target error) bool { return target == ErrGuardianMatch }

type AdmissionGuardian struct {
	// Either GuardianID (an existing guardian) or New.
	GuardianID   string         `json:"guardian_id"`
	New          *GuardianInput `json:"new"`
	Relationship string         `json:"relationship"`
	IsPrimary    bool           `json:"is_primary"`
	// ConfirmNew creates the new guardian even though one with the same
	// phone or email exists (for example two families sharing a phone).
	ConfirmNew bool `json:"confirm_new"`
}
type AdmissionAccount struct {
	Create bool   `json:"create"`
	Email  string `json:"email"`
}
type AdmissionInput struct {
	// IdempotencyKey identifies this admission attempt (a UUID made by the
	// client and resent on retries). A retry returns the original admission.
	IdempotencyKey string              `json:"idempotency_key"`
	Student        StudentInput        `json:"student"`
	Account        AdmissionAccount    `json:"account"`
	Guardians      []AdmissionGuardian `json:"guardians"`
}
type AdmissionLogin struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	// InitialPassword is returned once, in this response only. It is never
	// stored in plain text, logged or written to the audit log.
	InitialPassword string `json:"initial_password"`
}
type AdmissionResult struct {
	Student   Student         `json:"student"`
	Guardians []Relationship  `json:"guardians"`
	Login     *AdmissionLogin `json:"login,omitempty"`
	// Replayed: this request repeated an admission already made with the same
	// key. Nothing new was created and the initial password is not shown again.
	Replayed bool `json:"replayed"`
}

// admissionRecord is what an earlier request with the same key admitted.
type admissionRecord struct {
	StudentID   string
	AdmissionNo string
	UserID      string
	LoginEmail  string
}
type GuardianMatch struct {
	Guardian
	MatchedOn string   `json:"matched_on"`
	Children  []string `json:"children"`
}
type AdmissionDefaults struct {
	NextRoll    int    `json:"next_roll"`
	AdmissionNo string `json:"admission_no"`
	LoginDomain string `json:"login_domain"`
}

// admitRecord is what the repository writes; the password is already hashed.
type admitRecord struct {
	AdmissionInput
	PasswordHash string
}

// PhoneDigits keeps the digits of a phone number (Bangla digits become ASCII)
// and drops a leading Bangladesh country code, so "+880 1711-000000" and
// "01711000000" compare equal.
func PhoneDigits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '০' && r <= '৯':
			b.WriteRune('0' + (r - '০'))
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "880") && len(d) == 13 {
		d = d[2:]
	}
	return d
}

func (s *Service) Admit(ctx context.Context, a Actor, in AdmissionInput) (AdmissionResult, error) {
	if !validID(a.ID) || !validID(in.IdempotencyKey) {
		return AdmissionResult{}, ErrInvalid
	}
	in.IdempotencyKey = strings.ToLower(in.IdempotencyKey)
	if in.Student.Status == "" {
		in.Student.Status = "active"
	}
	// A login is created here or linked later on the student's page.
	if in.Student.UserID != "" || len(in.Guardians) > 2 {
		return AdmissionResult{}, ErrInvalid
	}
	student, err := normalizeStudent(in.Student)
	if err != nil {
		return AdmissionResult{}, err
	}
	in.Student = student
	// A retry of an admission already made returns it before any work.
	if out, found, err := s.replay(ctx, a, in); found || err != nil {
		return out, err
	}
	primary, seen := 0, map[string]bool{}
	for i := range in.Guardians {
		g := &in.Guardians[i]
		g.Relationship = strings.TrimSpace(g.Relationship)
		if !validText(g.Relationship, 60, true) || (g.GuardianID == "") == (g.New == nil) {
			return AdmissionResult{}, ErrInvalid
		}
		if g.GuardianID != "" {
			if !validID(g.GuardianID) || seen[strings.ToLower(g.GuardianID)] {
				return AdmissionResult{}, ErrInvalid
			}
			seen[strings.ToLower(g.GuardianID)] = true
		} else {
			v, err := normalizeGuardian(*g.New)
			if err != nil || v.UserID != "" {
				return AdmissionResult{}, ErrInvalid
			}
			g.New = &v
		}
		if g.IsPrimary {
			primary++
		}
	}
	if primary > 1 {
		return AdmissionResult{}, ErrInvalid
	}
	if primary == 0 && len(in.Guardians) > 0 {
		in.Guardians[0].IsPrimary = true
	}
	record := admitRecord{AdmissionInput: in}
	var password string
	if in.Account.Create {
		email, err := auth.NormalizeEmail(in.Account.Email)
		if err != nil {
			return AdmissionResult{}, ErrInvalid
		}
		record.Account.Email = email
		if password, err = auth.TemporaryPassword(); err != nil {
			return AdmissionResult{}, err
		}
		if record.PasswordHash, err = auth.HashPassword(password); err != nil {
			return AdmissionResult{}, err
		}
	} else {
		record.Account = AdmissionAccount{}
	}
	id, userID, err := s.repo.Admit(ctx, a, record)
	if errors.Is(err, errAdmissionKeyTaken) {
		// The same key was admitted by a request running at the same time.
		if out, found, err := s.replay(ctx, a, in); found || err != nil {
			return out, err
		}
	}
	if err != nil {
		return AdmissionResult{}, err
	}
	out := AdmissionResult{Guardians: []Relationship{}}
	if out.Student, err = s.repo.GetStudent(ctx, a, id); err != nil {
		return AdmissionResult{}, err
	}
	if out.Guardians, err = s.repo.ListRelationships(ctx, id); err != nil {
		return AdmissionResult{}, err
	}
	if userID != "" {
		out.Login = &AdmissionLogin{UserID: userID, Email: record.Account.Email, InitialPassword: password}
	}
	return out, nil
}

// replay returns the admission an earlier request with the same key made:
// the student, the guardians and the login's email, never its password.
func (s *Service) replay(ctx context.Context, a Actor, in AdmissionInput) (AdmissionResult, bool, error) {
	done, found, err := s.repo.FindAdmission(ctx, a, in.IdempotencyKey)
	if err != nil || !found {
		return AdmissionResult{}, false, err
	}
	if !strings.EqualFold(done.AdmissionNo, in.Student.AdmissionNo) {
		return AdmissionResult{}, false, ErrAdmissionKeyReused
	}
	out := AdmissionResult{Replayed: true}
	if out.Student, err = s.repo.GetStudent(ctx, a, done.StudentID); err != nil {
		return AdmissionResult{}, false, err
	}
	if out.Guardians, err = s.repo.ListRelationships(ctx, done.StudentID); err != nil {
		return AdmissionResult{}, false, err
	}
	if done.UserID != "" {
		out.Login = &AdmissionLogin{UserID: done.UserID, Email: done.LoginEmail}
	}
	return out, true, nil
}

// GuardianMatches lists guardians with the same phone (compared by digits) or
// email, for the duplicate check before a new guardian is created.
func (s *Service) GuardianMatches(ctx context.Context, phone, email string) ([]GuardianMatch, error) {
	phone, email = PhoneDigits(phone), strings.ToLower(strings.TrimSpace(email))
	if len(phone) > 32 || len(email) > 254 {
		return nil, ErrInvalid
	}
	if len(phone) < 6 {
		phone = ""
	}
	if phone == "" && email == "" {
		return []GuardianMatch{}, nil
	}
	return s.repo.GuardianMatches(ctx, phone, email)
}

func (s *Service) AdmissionDefaults(ctx context.Context, a Actor, sectionID string) (AdmissionDefaults, error) {
	if !validID(a.ID) || (sectionID != "" && !validID(sectionID)) {
		return AdmissionDefaults{}, ErrInvalid
	}
	return s.repo.AdmissionDefaults(ctx, a, sectionID)
}

// NextAdmissionNumber continues the school's numbering when the recent
// admission numbers (newest first) share a prefix and end in digits, such as
// ADM-2026-0041 → ADM-2026-0042. It returns "" when there is no pattern.
// taken reports numbers already in use; existing holds every number that has
// the prefix, so the next one is above the highest.
func NextAdmissionNumber(recent, existing []string, taken func(string) bool) string {
	if len(recent) < 2 {
		return ""
	}
	first := admissionNumberShape.FindStringSubmatch(recent[0])
	if first == nil {
		return ""
	}
	prefix, width, same := first[1], len(first[2]), 0
	for _, r := range recent {
		if m := admissionNumberShape.FindStringSubmatch(r); m != nil && m[1] == prefix {
			same++
		}
	}
	if same < 2 {
		return ""
	}
	highest := 0
	for _, r := range append(append([]string{}, recent...), existing...) {
		m := admissionNumberShape.FindStringSubmatch(r)
		if m == nil || !strings.EqualFold(m[1], prefix) || len(m[2]) > 9 {
			continue
		}
		if n, err := strconv.Atoi(m[2]); err == nil && n > highest {
			highest = n
		}
	}
	for n := highest + 1; n < highest+50; n++ {
		digits := strconv.Itoa(n)
		if len(digits) < width {
			digits = strings.Repeat("0", width-len(digits)) + digits
		}
		if candidate := prefix + digits; !taken(candidate) {
			return candidate
		}
	}
	return ""
}

// admissionError names which unique value an admission collided with.
func admissionError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		switch pg.ConstraintName {
		case "users_email_key":
			return ErrLoginTaken
		case "students_admission_key":
			return ErrAdmissionNoTaken
		case "students_section_roll_key", "student_enrolments_roll_key":
			return ErrRollTaken
		case "admission_requests_pkey":
			return errAdmissionKeyTaken
		}
	}
	return databaseError(err)
}

func guardianMatches(tx *gorm.DB, phone, email string) ([]GuardianMatch, error) {
	var rows []GuardianMatch
	q := tx.Table("guardians").Select("id,name_en,name_bn,phone,email,address,user_id, CASE WHEN ? <> '' AND right(regexp_replace(translate(phone,'০১২৩৪৫৬৭৮৯','0123456789'),'\\D','','g'),10) = right(?,10) THEN 'phone' ELSE 'email' END AS matched_on", phone, phone)
	switch {
	case phone != "" && email != "":
		q = q.Where("right(regexp_replace(translate(phone,'০১২৩৪৫৬৭৮৯','0123456789'),'\\D','','g'),10) = right(?,10) OR (email <> '' AND lower(email) = ?)", phone, email)
	case phone != "":
		q = q.Where("right(regexp_replace(translate(phone,'০১২৩৪৫৬৭৮৯','0123456789'),'\\D','','g'),10) = right(?,10)", phone)
	default:
		q = q.Where("email <> '' AND lower(email) = ?", email)
	}
	if err := q.Order("name_en,id").Limit(10).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Children = []string{}
		if err := tx.Table("student_guardians AS sg").Joins("JOIN students AS s ON s.id=sg.student_id").Where("sg.guardian_id=?", rows[i].ID).Order("s.name_en").Limit(5).Pluck("s.name_en", &rows[i].Children).Error; err != nil {
			return nil, err
		}
	}
	if rows == nil {
		rows = []GuardianMatch{}
	}
	return rows, nil
}

func (r *GormRepository) GuardianMatches(ctx context.Context, phone, email string) ([]GuardianMatch, error) {
	return guardianMatches(r.db.WithContext(ctx), phone, email)
}

func (r *GormRepository) Admit(ctx context.Context, a Actor, in admitRecord) (string, string, error) {
	var studentID, userID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		if len(in.Guardians) > 0 {
			if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
				return err
			}
		}
		if in.Account.Create {
			if err := requireAdmin(tx, a, "users:create"); err != nil {
				return err
			}
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		// Duplicate guardians are refused before anything is written.
		for i, g := range in.Guardians {
			if g.New == nil || g.ConfirmNew {
				continue
			}
			matches, err := guardianMatches(tx, PhoneDigits(g.New.Phone), g.New.Email)
			if err != nil {
				return err
			}
			if len(matches) > 0 {
				return &GuardianMatchError{Index: i, Matches: matches}
			}
		}
		student := in.Student
		if in.Account.Create {
			// The student sets their own password at the first sign-in.
			if err := tx.Raw("INSERT INTO users (email,display_name,role,is_active,password_hash,must_change_password) VALUES (?,?,'student',true,?,true) RETURNING id", in.Account.Email, student.NameEN, in.PasswordHash).Scan(&userID).Error; err != nil {
				return err
			}
			if err := audit.Record(tx, &a.ID, "user.created", "user", userID, map[string]any{"role": "student", "source": "admission"}); err != nil {
				return err
			}
			student.UserID = userID
		}
		var err error
		if studentID, err = saveStudentTx(tx, "", student); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO admission_requests (idempotency_key,student_id,admission_no,created_by) VALUES (?,?,?,?)", in.IdempotencyKey, studentID, student.AdmissionNo, a.ID).Error; err != nil {
			return err
		}
		created := 0
		for _, g := range in.Guardians {
			guardianID := g.GuardianID
			if g.New != nil {
				v := g.New
				if err := tx.Raw("INSERT INTO guardians (name_en,name_bn,phone,email,address) VALUES (?,?,?,?,?) RETURNING id", v.NameEN, v.NameBN, v.Phone, v.Email, v.Address).Scan(&guardianID).Error; err != nil {
					return err
				}
				if err := audit.Record(tx, &a.ID, "guardian.saved", "guardian", guardianID, map[string]any{"created": true, "source": "admission"}); err != nil {
					return err
				}
				created++
			}
			if err := tx.Exec("INSERT INTO student_guardians (student_id,guardian_id,relationship,is_primary) VALUES (?,?,?,?)", studentID, guardianID, g.Relationship, g.IsPrimary).Error; err != nil {
				return err
			}
		}
		return audit.Record(tx, &a.ID, "student.admitted", "student", studentID, map[string]any{"login_created": in.Account.Create, "guardians_created": created, "guardians_linked": len(in.Guardians) - created})
	})
	if err != nil {
		var match *GuardianMatchError
		if errors.As(err, &match) {
			return "", "", match
		}
		return "", "", admissionError(err)
	}
	return studentID, userID, nil
}

func (r *GormRepository) FindAdmission(ctx context.Context, a Actor, key string) (admissionRecord, bool, error) {
	db := r.db.WithContext(ctx)
	if err := requireAdmin(db, a, "students:manage"); err != nil {
		return admissionRecord{}, false, err
	}
	var rows []admissionRecord
	err := db.Raw("SELECT ar.student_id, ar.admission_no, COALESCE(s.user_id::text,'') AS user_id, COALESCE(u.email,'') AS login_email FROM admission_requests ar JOIN students s ON s.id=ar.student_id LEFT JOIN users u ON u.id=s.user_id WHERE ar.idempotency_key=?", key).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return admissionRecord{}, false, err
	}
	return rows[0], true, nil
}

func (r *GormRepository) AdmissionDefaults(ctx context.Context, a Actor, sectionID string) (AdmissionDefaults, error) {
	db := r.db.WithContext(ctx)
	if err := requireAdmin(db, a, "students:manage"); err != nil {
		return AdmissionDefaults{}, err
	}
	out := AdmissionDefaults{NextRoll: 1}
	if sectionID != "" {
		if err := db.Raw("SELECT GREATEST((SELECT COALESCE(MAX(roll_no),0) FROM students WHERE section_id=?),(SELECT COALESCE(MAX(roll_no),0) FROM student_enrolments WHERE section_id=?))+1", sectionID, sectionID).Scan(&out.NextRoll).Error; err != nil {
			return AdmissionDefaults{}, err
		}
	}
	var recent []string
	if err := db.Table("students").Order("created_at DESC, id DESC").Limit(5).Pluck("admission_no", &recent).Error; err != nil {
		return AdmissionDefaults{}, err
	}
	var existing []string
	if len(recent) > 0 {
		if m := admissionNumberShape.FindStringSubmatch(recent[0]); m != nil {
			pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(m[1])) + "%"
			if err := db.Table("students").Where("lower(admission_no) LIKE ?", pattern).Limit(20000).Pluck("admission_no", &existing).Error; err != nil {
				return AdmissionDefaults{}, err
			}
		}
	}
	used := map[string]bool{}
	for _, v := range existing {
		used[strings.ToLower(v)] = true
	}
	out.AdmissionNo = NextAdmissionNumber(recent, existing, func(v string) bool { return used[strings.ToLower(v)] })
	var email string
	if err := db.Table("users").Where("id=?", a.ID).Pluck("email", &email).Error; err != nil {
		return AdmissionDefaults{}, err
	}
	if at := strings.LastIndexByte(email, '@'); at >= 0 {
		out.LoginDomain = email[at+1:]
	}
	return out, nil
}
