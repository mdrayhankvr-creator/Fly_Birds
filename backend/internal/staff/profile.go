package staff

import (
	"context"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Staff profiles (checkpoint 4). Every signed-in user may open one; the
// server removes each field the viewer may not see before it is sent:
//
//   - always: photo, name, designation, subjects and classes taught, today's
//     routine;
//   - phone, email, qualifications: administrators, the head teacher and the
//     employee always; other staff and students/families only when the
//     employee allows it for that audience;
//   - joining date, attendance and leave: administrators, the head teacher
//     and the employee;
//   - MPO/index number: administrators, the head teacher and the employee;
//   - NID: administrators and the employee, masked; the full number only
//     through RevealNID, which is audited.

var (
	nidPattern = regexp.MustCompile(`^([0-9]{10}|[0-9]{13}|[0-9]{17})?$`)
	Dhaka      = mustZone("Asia/Dhaka")
)

func mustZone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("BST", 6*60*60)
	}
	return zone
}

// Viewer is the signed-in user opening a profile.
type Viewer struct{ ID, Role string }

// Audience is who a contact detail is shared with beyond the school's
// administrators and head teacher.
type Audience struct {
	Staff    bool `json:"staff"`
	Families bool `json:"families"`
}
type Visibility struct {
	Phone          Audience `json:"phone"`
	Email          Audience `json:"email"`
	Qualifications Audience `json:"qualifications"`
}
type Teaching struct {
	SubjectEN string `json:"subject_en"`
	SubjectBN string `json:"subject_bn"`
	ClassName string `json:"class_name"`
}
type Period struct {
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
	SubjectEN   string `json:"subject_en"`
	SubjectBN   string `json:"subject_bn"`
	ClassName   string `json:"class_name"`
	SectionName string `json:"section_name"`
	Room        string `json:"room"`
}
type AttendanceCounts struct {
	Present int `json:"present"`
	Absent  int `json:"absent"`
	Leave   int `json:"leave"`
}
type StaffAttendance struct {
	Month   AttendanceCounts `json:"month"`
	Session AttendanceCounts `json:"session"`
}

// Profile is one employee as a particular viewer may see them. A nil or
// omitted field is one the viewer may not see.
type Profile struct {
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	NameEN         string           `json:"name_en"`
	NameBN         string           `json:"name_bn"`
	Designation    string           `json:"designation"`
	IsActive       bool             `json:"is_active"`
	PhotoVersion   string           `json:"photo_version"`
	Teaching       []Teaching       `json:"teaching"`
	Today          []Period         `json:"today"`
	Phone          *string          `json:"phone,omitempty"`
	Email          *string          `json:"email,omitempty"`
	Qualifications []string         `json:"qualifications,omitempty"`
	JoinedOn       *string          `json:"joined_on,omitempty"`
	MPOIndex       *string          `json:"mpo_index,omitempty"`
	NIDMasked      *string          `json:"nid_masked,omitempty"`
	Attendance     *StaffAttendance `json:"attendance,omitempty"`
	Leaves         []Leave          `json:"leaves,omitempty"`
	Visibility     *Visibility      `json:"visibility,omitempty"`
	// What the viewer may do here.
	Self            bool `json:"self"`
	CanEditOwn      bool `json:"can_edit_own"`
	CanEditOfficial bool `json:"can_edit_official"`
	CanRevealNID    bool `json:"can_reveal_nid"`
	CanSeeSalary    bool `json:"can_see_salary"`
}

// OwnProfileInput is what employees change themselves.
type OwnProfileInput struct {
	Qualifications []string   `json:"qualifications"`
	Visibility     Visibility `json:"visibility"`
}

// OfficialInput is what administrators change. A nil field is left as it is.
type OfficialInput struct {
	Email    *string `json:"email"`
	MPOIndex *string `json:"mpo_index"`
	NID      *string `json:"nid"`
}

// profileRow is everything stored about an employee for a profile.
type profileRow struct {
	ID                       string  `gorm:"column:id"`
	Kind                     string  `gorm:"column:kind"`
	NameEN                   string  `gorm:"column:name_en"`
	NameBN                   string  `gorm:"column:name_bn"`
	Designation              string  `gorm:"column:designation"`
	Phone                    string  `gorm:"column:phone"`
	Email                    string  `gorm:"column:email"`
	JoinedOn                 string  `gorm:"column:joined_on"`
	UserID                   *string `gorm:"column:user_id"`
	IsActive                 bool    `gorm:"column:is_active"`
	Qualifications           string  `gorm:"column:qualifications"`
	PhoneToStaff             bool    `gorm:"column:phone_to_staff"`
	PhoneToFamilies          bool    `gorm:"column:phone_to_families"`
	EmailToStaff             bool    `gorm:"column:email_to_staff"`
	EmailToFamilies          bool    `gorm:"column:email_to_families"`
	QualificationsToStaff    bool    `gorm:"column:qualifications_to_staff"`
	QualificationsToFamilies bool    `gorm:"column:qualifications_to_families"`
	MPOIndex                 string  `gorm:"column:mpo_index"`
	NID                      string  `gorm:"column:nid"`
	PhotoVersion             string  `gorm:"column:photo_version"`
}

type ProfileRepository interface {
	ProfileRow(ctx context.Context, id string) (profileRow, error)
	OwnEmployeeID(ctx context.Context, user string) (string, error)
	Teaching(ctx context.Context, id string) ([]Teaching, error)
	TodayPeriods(ctx context.Context, id string, weekday int) ([]Period, error)
	StaffAttendance(ctx context.Context, id string, today time.Time) (StaffAttendance, error)
	RecentLeaves(ctx context.Context, id string) ([]Leave, error)
	SaveOwnProfile(ctx context.Context, actor, id string, in OwnProfileInput) error
	SaveOfficial(ctx context.Context, actor, id string, in OfficialInput) error
	RevealNID(ctx context.Context, actor, id string, self bool) (string, error)
	SavePhoto(ctx context.Context, actor, id string, content []byte, version string) error
	DeletePhoto(ctx context.Context, actor, id string) error
	Photo(ctx context.Context, id string) ([]byte, string, error)
}

func (s *Service) profiles() (ProfileRepository, error) {
	repo, ok := s.repo.(ProfileRepository)
	if !ok {
		return nil, ErrNotFound
	}
	return repo, nil
}

// MaskNID keeps the last three digits: "1234567890" → "•••••••890".
func MaskNID(nid string) string {
	if nid == "" {
		return ""
	}
	if len(nid) <= 3 {
		return strings.Repeat("•", len(nid))
	}
	return strings.Repeat("•", len(nid)-3) + nid[len(nid)-3:]
}

func isSelf(row profileRow, v Viewer) bool {
	return row.UserID != nil && strings.EqualFold(*row.UserID, v.ID)
}

// shared reports whether a contact detail reaches this viewer.
func shared(a Audience, v Viewer) bool {
	switch v.Role {
	case "teacher", "accountant":
		return a.Staff
	case "student", "guardian":
		return a.Families
	}
	return false
}

func (s *Service) Profile(ctx context.Context, v Viewer, id string) (Profile, error) {
	repo, err := s.profiles()
	if err != nil {
		return Profile{}, err
	}
	if !uuid.MatchString(v.ID) || !uuid.MatchString(id) {
		return Profile{}, ErrInvalid
	}
	row, err := repo.ProfileRow(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	self := isSelf(row, v)
	admin, head := v.Role == "admin", v.Role == "head_teacher"
	// Students and families see only the school's current staff.
	if !row.IsActive && !(admin || head || self) {
		return Profile{}, ErrNotFound
	}
	now := time.Now().In(Dhaka)
	p := Profile{ID: row.ID, Kind: row.Kind, NameEN: row.NameEN, NameBN: row.NameBN, Designation: row.Designation, IsActive: row.IsActive, PhotoVersion: row.PhotoVersion,
		Self: self, CanEditOwn: self, CanEditOfficial: admin, CanRevealNID: (admin || self) && row.NID != "", CanSeeSalary: admin}
	if p.Teaching, err = repo.Teaching(ctx, id); err != nil {
		return Profile{}, err
	}
	if p.Today, err = repo.TodayPeriods(ctx, id, int(now.Weekday())); err != nil {
		return Profile{}, err
	}
	var qualifications []string
	_ = json.Unmarshal([]byte(row.Qualifications), &qualifications)
	visibility := Visibility{Phone: Audience{row.PhoneToStaff, row.PhoneToFamilies}, Email: Audience{row.EmailToStaff, row.EmailToFamilies}, Qualifications: Audience{row.QualificationsToStaff, row.QualificationsToFamilies}}
	full := admin || head || self
	if full || shared(visibility.Phone, v) {
		p.Phone = &row.Phone
	}
	if full || shared(visibility.Email, v) {
		p.Email = &row.Email
	}
	if full || shared(visibility.Qualifications, v) {
		if qualifications == nil {
			qualifications = []string{}
		}
		p.Qualifications = qualifications
	}
	if full {
		p.JoinedOn, p.MPOIndex = &row.JoinedOn, &row.MPOIndex
		attendance, err := repo.StaffAttendance(ctx, id, now)
		if err != nil {
			return Profile{}, err
		}
		p.Attendance = &attendance
		if p.Leaves, err = repo.RecentLeaves(ctx, id); err != nil {
			return Profile{}, err
		}
	}
	if admin || self {
		masked := MaskNID(row.NID)
		p.NIDMasked = &masked
		p.Visibility = &visibility
	}
	return p, nil
}

// OwnProfileID is the employee record of the signed-in user.
func (s *Service) OwnProfileID(ctx context.Context, v Viewer) (string, error) {
	repo, err := s.profiles()
	if err != nil {
		return "", err
	}
	if !uuid.MatchString(v.ID) {
		return "", ErrInvalid
	}
	return repo.OwnEmployeeID(ctx, v.ID)
}

func (s *Service) SaveOwnProfile(ctx context.Context, v Viewer, in OwnProfileInput) (Profile, error) {
	repo, err := s.profiles()
	if err != nil {
		return Profile{}, err
	}
	id, err := s.OwnProfileID(ctx, v)
	if err != nil {
		return Profile{}, err
	}
	clean := make([]string, 0, len(in.Qualifications))
	for _, q := range in.Qualifications {
		q = strings.Join(strings.Fields(q), " ")
		if q == "" {
			continue
		}
		if !utf8.ValidString(q) || len(q) > 160 {
			return Profile{}, ErrInvalid
		}
		clean = append(clean, q)
	}
	if len(clean) > 20 {
		return Profile{}, ErrInvalid
	}
	in.Qualifications = clean
	if err := repo.SaveOwnProfile(ctx, v.ID, id, in); err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, v, id)
}

func (s *Service) SaveOfficial(ctx context.Context, v Viewer, id string, in OfficialInput) (Profile, error) {
	repo, err := s.profiles()
	if err != nil {
		return Profile{}, err
	}
	if !uuid.MatchString(v.ID) || !uuid.MatchString(id) || v.Role != "admin" {
		return Profile{}, ErrForbidden
	}
	if in.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*in.Email))
		if e != "" {
			if parsed, err := mail.ParseAddress(e); err != nil || parsed.Address != e || len(e) > 254 {
				return Profile{}, ErrInvalid
			}
		}
		in.Email = &e
	}
	if in.MPOIndex != nil {
		m := strings.TrimSpace(*in.MPOIndex)
		if !utf8.ValidString(m) || len(m) > 40 {
			return Profile{}, ErrInvalid
		}
		in.MPOIndex = &m
	}
	if in.NID != nil {
		n := strings.ReplaceAll(strings.TrimSpace(*in.NID), " ", "")
		if !nidPattern.MatchString(n) {
			return Profile{}, ErrInvalid
		}
		in.NID = &n
	}
	if err := repo.SaveOfficial(ctx, v.ID, id, in); err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, v, id)
}

// RevealNID returns the full NID to an administrator or the employee and
// records that it was shown.
func (s *Service) RevealNID(ctx context.Context, v Viewer, id string) (string, error) {
	repo, err := s.profiles()
	if err != nil {
		return "", err
	}
	if !uuid.MatchString(v.ID) || !uuid.MatchString(id) {
		return "", ErrInvalid
	}
	row, err := repo.ProfileRow(ctx, id)
	if err != nil {
		return "", err
	}
	self := isSelf(row, v)
	if v.Role != "admin" && !self {
		return "", ErrForbidden
	}
	if row.NID == "" {
		return "", ErrNotFound
	}
	return repo.RevealNID(ctx, v.ID, id, self)
}

// SavePhoto stores an employee's photo: the employee's own, or any as an
// administrator. The upload is cropped square, resized and re-encoded.
func (s *Service) SavePhoto(ctx context.Context, v Viewer, id, contentType string, data []byte) (Profile, error) {
	repo, err := s.profiles()
	if err != nil {
		return Profile{}, err
	}
	if err := s.mayChangePhoto(ctx, repo, v, id); err != nil {
		return Profile{}, err
	}
	content, version, err := ProcessPhoto(contentType, data)
	if err != nil {
		return Profile{}, err
	}
	if err := repo.SavePhoto(ctx, v.ID, id, content, version); err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, v, id)
}

func (s *Service) DeletePhoto(ctx context.Context, v Viewer, id string) error {
	repo, err := s.profiles()
	if err != nil {
		return err
	}
	if err := s.mayChangePhoto(ctx, repo, v, id); err != nil {
		return err
	}
	return repo.DeletePhoto(ctx, v.ID, id)
}

func (s *Service) mayChangePhoto(ctx context.Context, repo ProfileRepository, v Viewer, id string) error {
	if !uuid.MatchString(v.ID) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	row, err := repo.ProfileRow(ctx, id)
	if err != nil {
		return err
	}
	if v.Role != "admin" && !isSelf(row, v) {
		return ErrForbidden
	}
	return nil
}

// Photo returns an employee's photo to a viewer allowed to open the profile.
func (s *Service) Photo(ctx context.Context, v Viewer, id string) ([]byte, string, error) {
	repo, err := s.profiles()
	if err != nil {
		return nil, "", err
	}
	if _, err := s.Profile(ctx, v, id); err != nil {
		return nil, "", err
	}
	return repo.Photo(ctx, id)
}

// ---- repository ----

func (r *GormRepository) ProfileRow(ctx context.Context, id string) (profileRow, error) {
	var row profileRow
	err := r.db.WithContext(ctx).Raw(`SELECT e.id, e.kind, e.name_en, e.name_bn, e.designation, e.phone, e.email, to_char(e.joined_on,'YYYY-MM-DD') AS joined_on, e.user_id, e.is_active,
  e.qualifications::text AS qualifications, e.phone_to_staff, e.phone_to_families, e.email_to_staff, e.email_to_families, e.qualifications_to_staff, e.qualifications_to_families,
  COALESCE(i.mpo_index,'') AS mpo_index, COALESCE(i.nid,'') AS nid, COALESCE(p.version,'') AS photo_version
FROM employees e LEFT JOIN employee_identity i ON i.employee_id = e.id LEFT JOIN employee_photos p ON p.employee_id = e.id WHERE e.id = ?`, id).Scan(&row).Error
	if err != nil {
		return profileRow{}, err
	}
	if row.ID == "" {
		return profileRow{}, ErrNotFound
	}
	return row, nil
}

func (r *GormRepository) OwnEmployeeID(ctx context.Context, user string) (string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Table("employees").Where("user_id = ?", user).Pluck("id", &ids).Error; err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", ErrNotFound
	}
	return ids[0], nil
}

// Teaching lists the subjects and classes taught in the active session.
func (r *GormRepository) Teaching(ctx context.Context, id string) ([]Teaching, error) {
	out := []Teaching{}
	err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT sub.name_en AS subject_en, sub.name_bn AS subject_bn, c.name AS class_name, c.display_order
FROM teacher_assignments a JOIN academic_sessions se ON se.id = a.session_id AND se.is_active JOIN academic_subjects sub ON sub.id = a.subject_id JOIN academic_classes c ON c.id = a.class_id
WHERE a.employee_id = ? ORDER BY c.display_order, c.name, sub.name_en`, id).Scan(&out).Error
	return out, err
}

// TodayPeriods lists the employee's class periods today in the active session.
func (r *GormRepository) TodayPeriods(ctx context.Context, id string, weekday int) ([]Period, error) {
	out := []Period{}
	err := r.db.WithContext(ctx).Raw(`SELECT r.start_minute, r.end_minute, sub.name_en AS subject_en, sub.name_bn AS subject_bn, c.name AS class_name, sec.name AS section_name, r.room
FROM class_routines r JOIN academic_sessions se ON se.id = r.session_id AND se.is_active JOIN academic_subjects sub ON sub.id = r.subject_id JOIN academic_classes c ON c.id = r.class_id JOIN academic_sections sec ON sec.id = r.section_id
WHERE r.teacher_id = ? AND r.day_of_week = ? ORDER BY r.start_minute, r.id`, id, weekday).Scan(&out).Error
	return out, err
}

func (r *GormRepository) StaffAttendance(ctx context.Context, id string, today time.Time) (StaffAttendance, error) {
	var out StaffAttendance
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	err := r.db.WithContext(ctx).Raw(`SELECT
  count(*) FILTER (WHERE a.status = 'present' AND a.attendance_date >= ?::date) AS month_present,
  count(*) FILTER (WHERE a.status = 'absent' AND a.attendance_date >= ?::date) AS month_absent,
  count(*) FILTER (WHERE a.status = 'leave' AND a.attendance_date >= ?::date) AS month_leave,
  count(*) FILTER (WHERE a.status = 'present') AS session_present,
  count(*) FILTER (WHERE a.status = 'absent') AS session_absent,
  count(*) FILTER (WHERE a.status = 'leave') AS session_leave
FROM teacher_attendance a JOIN academic_sessions se ON se.id = a.session_id AND se.is_active WHERE a.employee_id = ?`, month, month, month, id).Row().Scan(&out.Month.Present, &out.Month.Absent, &out.Month.Leave, &out.Session.Present, &out.Session.Absent, &out.Session.Leave)
	return out, err
}

func (r *GormRepository) RecentLeaves(ctx context.Context, id string) ([]Leave, error) {
	out := []Leave{}
	err := r.db.WithContext(ctx).Table("employee_leaves").Select("id,employee_id,to_char(starts_on,'YYYY-MM-DD') AS starts_on,to_char(ends_on,'YYYY-MM-DD') AS ends_on,leave_type,reason,status,reviewed_by").Where("employee_id = ?", id).Order("starts_on DESC, id").Limit(5).Scan(&out).Error
	return out, err
}

func (r *GormRepository) SaveOwnProfile(ctx context.Context, actor, id string, in OwnProfileInput) error {
	qualifications, err := json.Marshal(in.Qualifications)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v := in.Visibility
		res := tx.Exec(`UPDATE employees SET qualifications = ?::jsonb, phone_to_staff = ?, phone_to_families = ?, email_to_staff = ?, email_to_families = ?, qualifications_to_staff = ?, qualifications_to_families = ?, updated_at = now() WHERE id = ? AND user_id = ?`,
			string(qualifications), v.Phone.Staff, v.Phone.Families, v.Email.Staff, v.Email.Families, v.Qualifications.Staff, v.Qualifications.Families, id, actor)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrForbidden
		}
		return audit.Record(tx, &actor, "staff.profile_saved", "employee", id, map[string]any{"qualifications": len(in.Qualifications), "visibility": v})
	})
}

func (r *GormRepository) SaveOfficial(ctx context.Context, actor, id string, in OfficialInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdministrator(tx, actor, "staff:manage"); err != nil {
			return err
		}
		fields := []string{}
		if in.Email != nil {
			res := tx.Exec("UPDATE employees SET email = ?, updated_at = now() WHERE id = ?", *in.Email, id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrNotFound
			}
			fields = append(fields, "email")
		}
		if in.MPOIndex != nil || in.NID != nil {
			if err := tx.Exec("INSERT INTO employee_identity (employee_id, updated_by) VALUES (?, ?) ON CONFLICT (employee_id) DO NOTHING", id, actor).Error; err != nil {
				return dbError(err)
			}
			if in.MPOIndex != nil {
				if err := tx.Exec("UPDATE employee_identity SET mpo_index = ?, updated_by = ?, updated_at = now() WHERE employee_id = ?", *in.MPOIndex, actor, id).Error; err != nil {
					return err
				}
				fields = append(fields, "mpo_index")
			}
			if in.NID != nil {
				if err := tx.Exec("UPDATE employee_identity SET nid = ?, updated_by = ?, updated_at = now() WHERE employee_id = ?", *in.NID, actor, id).Error; err != nil {
					return err
				}
				fields = append(fields, "nid")
			}
		}
		// The audit names the fields, never their values.
		return audit.Record(tx, &actor, "staff.official_saved", "employee", id, map[string]any{"fields": fields})
	})
}

func (r *GormRepository) RevealNID(ctx context.Context, actor, id string, self bool) (string, error) {
	var nid string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("employee_identity").Where("employee_id = ?", id).Pluck("nid", &nid).Error; err != nil {
			return err
		}
		if nid == "" {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "staff.nid_revealed", "employee", id, map[string]any{"own": self})
	})
	return nid, err
}

func (r *GormRepository) SavePhoto(ctx context.Context, actor, id string, content []byte, version string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO employee_photos (employee_id, content, version, updated_by) VALUES (?, ?, ?, ?)
ON CONFLICT (employee_id) DO UPDATE SET content = EXCLUDED.content, version = EXCLUDED.version, updated_by = EXCLUDED.updated_by, updated_at = now()`, id, content, version, actor).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "staff.photo_saved", "employee", id, map[string]any{"bytes": len(content)})
	})
}

func (r *GormRepository) DeletePhoto(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM employee_photos WHERE employee_id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "staff.photo_deleted", "employee", id, nil)
	})
}

func (r *GormRepository) Photo(ctx context.Context, id string) ([]byte, string, error) {
	var rows []struct {
		Content []byte
		Version string
	}
	if err := r.db.WithContext(ctx).Table("employee_photos").Select("content, version").Where("employee_id = ?", id).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", ErrNotFound
	}
	return rows[0].Content, rows[0].Version, nil
}

func requireAdministrator(tx *gorm.DB, actor, permission string) error {
	var n int64
	if err := tx.Table("users AS u").Joins("JOIN role_permissions rp ON rp.role_id = u.role").Where("u.id = ? AND u.is_active AND u.role = 'admin' AND rp.permission_id = ?", actor, permission).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return ErrForbidden
	}
	return nil
}
