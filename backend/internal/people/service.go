package people

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid people input")
	ErrNotFound  = errors.New("people record not found")
	ErrDuplicate = errors.New("people record already exists")
	ErrForbidden = errors.New("people access denied")
	uuid         = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type Actor struct{ ID, Role string }
type Student struct {
	ID          string  `json:"id"`
	AdmissionNo string  `json:"admission_no"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	DateOfBirth string  `json:"date_of_birth"`
	Status      string  `json:"status"`
	SessionID   string  `json:"session_id"`
	ClassID     string  `json:"class_id"`
	SectionID   string  `json:"section_id"`
	RollNo      *int    `json:"roll_no"`
	UserID      *string `json:"user_id"`
	SessionName string  `json:"session_name"`
	ClassName   string  `json:"class_name"`
	SectionName string  `json:"section_name"`
	HasPhoto    bool    `json:"has_photo"`
	// Profile details (migration 1601); empty when not recorded.
	StudentCode      string `json:"student_code"`
	Gender           string `json:"gender"`
	BloodGroup       string `json:"blood_group"`
	FatherNameEN     string `json:"father_name_en"`
	FatherNameBN     string `json:"father_name_bn"`
	MotherNameEN     string `json:"mother_name_en"`
	MotherNameBN     string `json:"mother_name_bn"`
	PresentAddress   string `json:"present_address"`
	PermanentAddress string `json:"permanent_address"`
	AdmissionDate    string `json:"admission_date"`
	// Primary guardian from the existing relationship; only on single-student reads.
	PrimaryGuardian *GuardianSummary `json:"primary_guardian,omitempty" gorm:"-"`
}
type GuardianSummary struct {
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
}
type StudentInput struct {
	AdmissionNo string `json:"admission_no"`
	NameEN      string `json:"name_en"`
	NameBN      string `json:"name_bn"`
	DateOfBirth string `json:"date_of_birth"`
	Status      string `json:"status"`
	SessionID   string `json:"session_id"`
	ClassID     string `json:"class_id"`
	SectionID   string `json:"section_id"`
	RollNo      *int   `json:"roll_no"`
	UserID      string `json:"user_id"`
	// Optional profile details. An omitted field keeps the stored value on
	// update (and is empty on create), so older clients cannot erase them.
	StudentCode      *string `json:"student_code,omitempty"`
	Gender           *string `json:"gender,omitempty"`
	BloodGroup       *string `json:"blood_group,omitempty"`
	FatherNameEN     *string `json:"father_name_en,omitempty"`
	FatherNameBN     *string `json:"father_name_bn,omitempty"`
	MotherNameEN     *string `json:"mother_name_en,omitempty"`
	MotherNameBN     *string `json:"mother_name_bn,omitempty"`
	PresentAddress   *string `json:"present_address,omitempty"`
	PermanentAddress *string `json:"permanent_address,omitempty"`
	AdmissionDate    *string `json:"admission_date,omitempty"`
}
type Guardian struct {
	ID      string  `json:"id"`
	NameEN  string  `json:"name_en"`
	NameBN  string  `json:"name_bn"`
	Phone   string  `json:"phone"`
	Email   string  `json:"email"`
	Address string  `json:"address"`
	UserID  *string `json:"user_id"`
}
type GuardianInput struct {
	NameEN  string `json:"name_en"`
	NameBN  string `json:"name_bn"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Address string `json:"address"`
	UserID  string `json:"user_id"`
}
type Relationship struct {
	StudentID    string `json:"student_id"`
	GuardianID   string `json:"guardian_id"`
	Relationship string `json:"relationship"`
	IsPrimary    bool   `json:"is_primary"`
	GuardianName string `json:"guardian_name"`
	// GuardianNameBN and Phone let the student profile show each guardian
	// without a second request.
	GuardianNameBN string `json:"guardian_name_bn"`
	Phone          string `json:"phone"`
}
type LinkInput struct {
	GuardianID   string `json:"guardian_id"`
	Relationship string `json:"relationship"`
	IsPrimary    bool   `json:"is_primary"`
}
type Query struct {
	Search, Status, SessionID, ClassID, SectionID string
	Page, PageSize                                int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
type Photo struct {
	ContentType string
	Content     []byte
}

type Repository interface {
	ListStudents(context.Context, Query) (Page[Student], error)
	GetStudent(context.Context, Actor, string) (Student, error)
	OwnStudent(context.Context, Actor) (Student, error)
	SaveStudent(context.Context, Actor, string, StudentInput) (Student, error)
	DeleteStudent(context.Context, Actor, string) error
	ImportStudents(context.Context, Actor, []StudentInput) (int, error)
	SavePhoto(context.Context, Actor, string, Photo) error
	Enrolments(context.Context, Actor, string) ([]Enrolment, error)
	GetPhoto(context.Context, Actor, string) (Photo, error)
	ListGuardians(context.Context, Query) (Page[Guardian], error)
	GetGuardian(context.Context, string) (Guardian, error)
	OwnGuardian(context.Context, Actor) (Guardian, error)
	SaveGuardian(context.Context, Actor, string, GuardianInput) (Guardian, error)
	DeleteGuardian(context.Context, Actor, string) error
	ListRelationships(context.Context, string) ([]Relationship, error)
	Link(context.Context, Actor, string, LinkInput) (Relationship, error)
	Unlink(context.Context, Actor, string, string) error
	Children(context.Context, Actor) ([]Student, error)
	GuardianStudents(context.Context, string) ([]Student, error)
	CardSettings(context.Context) (CardSettings, error)
	SaveCardSettings(context.Context, Actor, CardSettingsInput) (CardSettings, error)
	SaveSignature(context.Context, Actor, *Photo) error
	Signature(context.Context, Actor) (Photo, error)
	CardSchool(context.Context) (CardSchool, error)
	StudentCard(context.Context, Actor, string) (Card, error)
	SectionCards(context.Context, Actor, string) ([]Card, error)
	ReissueCard(context.Context, Actor, string) error
	VerifyCard(context.Context, string, string) (CardVerification, error)
	Admit(context.Context, Actor, admitRecord) (string, string, error)
	GuardianMatches(context.Context, string, string) ([]GuardianMatch, error)
	AdmissionDefaults(context.Context, Actor, string) (AdmissionDefaults, error)
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }
func validID(s string) bool               { return uuid.MatchString(s) }
func validText(s string, max int, required bool) bool {
	return (!required || s != "") && len(s) <= max && utf8.ValidString(s) && s == strings.TrimSpace(s)
}
func validQuery(q Query) bool {
	return q.Page > 0 && q.Page <= 1000000 && q.PageSize > 0 && q.PageSize <= 100 && len(q.Search) <= 160 && (q.Status == "" || validStatus(q.Status)) && (q.SessionID == "" || validID(q.SessionID)) && (q.ClassID == "" || validID(q.ClassID)) && (q.SectionID == "" || validID(q.SectionID))
}
func validStatus(s string) bool {
	return s == "active" || s == "inactive" || s == "graduated" || s == "transferred"
}
func normalizeStudent(v StudentInput) (StudentInput, error) {
	v.AdmissionNo, v.NameEN, v.NameBN = strings.TrimSpace(v.AdmissionNo), strings.TrimSpace(v.NameEN), strings.TrimSpace(v.NameBN)
	if !validText(v.AdmissionNo, 64, true) || !validText(v.NameEN, 160, true) || !validText(v.NameBN, 160, false) || !validStatus(v.Status) || !validID(v.SessionID) || !validID(v.ClassID) || !validID(v.SectionID) || (v.UserID != "" && !validID(v.UserID)) || (v.RollNo != nil && (*v.RollNo < 1 || *v.RollNo > 999999)) {
		return v, ErrInvalid
	}
	if v.DateOfBirth != "" {
		birthday, err := time.Parse("2006-01-02", v.DateOfBirth)
		if err != nil || birthday.After(time.Now().UTC()) {
			return v, ErrInvalid
		}
	}
	return normalizeProfile(v)
}

var bloodGroups = map[string]bool{"": true, "A+": true, "A-": true, "B+": true, "B-": true, "AB+": true, "AB-": true, "O+": true, "O-": true}

// normalizeProfile trims and validates the optional details; nil fields are
// left nil so the repository keeps their stored values.
func normalizeProfile(v StudentInput) (StudentInput, error) {
	texts := []struct {
		field *string
		max   int
	}{{v.StudentCode, 64}, {v.FatherNameEN, 160}, {v.FatherNameBN, 160}, {v.MotherNameEN, 160}, {v.MotherNameBN, 160}, {v.PresentAddress, 500}, {v.PermanentAddress, 500}}
	for _, t := range texts {
		if t.field == nil {
			continue
		}
		*t.field = strings.TrimSpace(*t.field)
		if !validText(*t.field, t.max, false) {
			return v, ErrInvalid
		}
	}
	if v.Gender != nil && *v.Gender != "" && *v.Gender != "male" && *v.Gender != "female" && *v.Gender != "other" {
		return v, ErrInvalid
	}
	if v.BloodGroup != nil && !bloodGroups[*v.BloodGroup] {
		return v, ErrInvalid
	}
	if v.AdmissionDate != nil && *v.AdmissionDate != "" {
		admitted, err := time.Parse("2006-01-02", *v.AdmissionDate)
		// An admission may be recorded ahead of term, but not years ahead.
		if err != nil || admitted.After(time.Now().UTC().AddDate(1, 0, 0)) || admitted.Before(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return v, ErrInvalid
		}
		if v.DateOfBirth != "" && *v.AdmissionDate < v.DateOfBirth {
			return v, ErrInvalid
		}
	}
	return v, nil
}
func normalizeGuardian(v GuardianInput) (GuardianInput, error) {
	v.NameEN, v.NameBN, v.Phone, v.Email, v.Address = strings.TrimSpace(v.NameEN), strings.TrimSpace(v.NameBN), strings.TrimSpace(v.Phone), strings.TrimSpace(v.Email), strings.TrimSpace(v.Address)
	if !validText(v.NameEN, 160, true) || !validText(v.NameBN, 160, false) || !validText(v.Phone, 32, false) || !validText(v.Email, 254, false) || !validText(v.Address, 500, false) || (v.UserID != "" && !validID(v.UserID)) {
		return v, ErrInvalid
	}
	if v.Email != "" {
		v.Email = strings.ToLower(v.Email)
		parsed, err := mail.ParseAddress(v.Email)
		if err != nil || parsed.Address != v.Email {
			return v, ErrInvalid
		}
	}
	return v, nil
}
func (s *Service) ListStudents(ctx context.Context, q Query) (Page[Student], error) {
	if !validQuery(q) {
		return Page[Student]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListStudents(ctx, q)
}
func (s *Service) GetStudent(ctx context.Context, a Actor, id string) (Student, error) {
	if !validID(id) {
		return Student{}, ErrInvalid
	}
	return s.repo.GetStudent(ctx, a, id)
}

// Enrolments returns a student's year-by-year history.
func (s *Service) Enrolments(ctx context.Context, a Actor, id string) ([]Enrolment, error) {
	if !validID(id) {
		return nil, ErrInvalid
	}
	return s.repo.Enrolments(ctx, a, id)
}
func (s *Service) OwnStudent(ctx context.Context, a Actor) (Student, error) {
	return s.repo.OwnStudent(ctx, a)
}
func (s *Service) SaveStudent(ctx context.Context, a Actor, id string, v StudentInput) (Student, error) {
	if !validID(a.ID) || (id != "" && !validID(id)) {
		return Student{}, ErrInvalid
	}
	v, err := normalizeStudent(v)
	if err != nil {
		return Student{}, err
	}
	return s.repo.SaveStudent(ctx, a, id, v)
}
func (s *Service) DeleteStudent(ctx context.Context, a Actor, id string) error {
	if !validID(a.ID) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteStudent(ctx, a, id)
}
func (s *Service) ListGuardians(ctx context.Context, q Query) (Page[Guardian], error) {
	if !validQuery(q) {
		return Page[Guardian]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListGuardians(ctx, q)
}
func (s *Service) GetGuardian(ctx context.Context, id string) (Guardian, error) {
	if !validID(id) {
		return Guardian{}, ErrInvalid
	}
	return s.repo.GetGuardian(ctx, id)
}
func (s *Service) OwnGuardian(ctx context.Context, a Actor) (Guardian, error) {
	return s.repo.OwnGuardian(ctx, a)
}
func (s *Service) SaveGuardian(ctx context.Context, a Actor, id string, v GuardianInput) (Guardian, error) {
	if !validID(a.ID) || (id != "" && !validID(id)) {
		return Guardian{}, ErrInvalid
	}
	v, err := normalizeGuardian(v)
	if err != nil {
		return Guardian{}, err
	}
	return s.repo.SaveGuardian(ctx, a, id, v)
}
func (s *Service) DeleteGuardian(ctx context.Context, a Actor, id string) error {
	if !validID(a.ID) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteGuardian(ctx, a, id)
}
func (s *Service) ListRelationships(ctx context.Context, id string) ([]Relationship, error) {
	if !validID(id) {
		return nil, ErrInvalid
	}
	return s.repo.ListRelationships(ctx, id)
}

// OwnGuardians lists the guardians linked to the signed-in student's own
// record. It never takes a student ID, so a student cannot read another
// student's guardians.
func (s *Service) OwnGuardians(ctx context.Context, a Actor) ([]Relationship, error) {
	own, err := s.repo.OwnStudent(ctx, a)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRelationships(ctx, own.ID)
}
func (s *Service) Link(ctx context.Context, a Actor, id string, v LinkInput) (Relationship, error) {
	v.Relationship = strings.TrimSpace(v.Relationship)
	if !validID(a.ID) || !validID(id) || !validID(v.GuardianID) || !validText(v.Relationship, 60, true) {
		return Relationship{}, ErrInvalid
	}
	return s.repo.Link(ctx, a, id, v)
}
func (s *Service) Unlink(ctx context.Context, a Actor, id, guardian string) error {
	if !validID(a.ID) || !validID(id) || !validID(guardian) {
		return ErrInvalid
	}
	return s.repo.Unlink(ctx, a, id, guardian)
}
func (s *Service) Children(ctx context.Context, a Actor) ([]Student, error) {
	return s.repo.Children(ctx, a)
}
func (s *Service) GuardianStudents(ctx context.Context, id string) ([]Student, error) {
	if !validID(id) {
		return nil, ErrInvalid
	}
	return s.repo.GuardianStudents(ctx, id)
}
func (s *Service) SavePhoto(ctx context.Context, a Actor, id string, p Photo) error {
	if !validID(id) || !validID(a.ID) || len(p.Content) < 1 || len(p.Content) > 2097152 {
		return ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(p.Content))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4000 || config.Height > 4000 || (format == "jpeg" && p.ContentType != "image/jpeg") || (format == "png" && p.ContentType != "image/png") || (format != "jpeg" && format != "png") {
		return ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(p.Content))
	if err != nil {
		return ErrInvalid
	}
	var clean bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&clean, decoded, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&clean, decoded)
	}
	if err != nil || clean.Len() > 2097152 {
		return ErrInvalid
	}
	p.Content = clean.Bytes()
	return s.repo.SavePhoto(ctx, a, id, p)
}
func (s *Service) GetPhoto(ctx context.Context, a Actor, id string) (Photo, error) {
	if !validID(id) {
		return Photo{}, ErrInvalid
	}
	return s.repo.GetPhoto(ctx, a, id)
}
func (s *Service) ImportCSV(ctx context.Context, a Actor, sessionID, classID, sectionID string, body []byte) (int, error) {
	if !validID(a.ID) || !validID(sessionID) || !validID(classID) || !validID(sectionID) || len(body) > 2097152 {
		return 0, ErrInvalid
	}
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	// PostgreSQL text cannot store NUL even though it is valid UTF-8.
	if bytes.IndexByte(body, 0) >= 0 {
		return 0, ErrInvalid
	}
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil || strings.Join(header, ",") != "admission_no,name_en,name_bn,date_of_birth,roll_no,status" {
		return 0, ErrInvalid
	}
	rows := make([]StudentInput, 0, 32)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(record) != 6 || len(rows) >= 1000 {
			return 0, ErrInvalid
		}
		var roll *int
		if strings.TrimSpace(record[4]) != "" {
			n, e := strconv.Atoi(record[4])
			if e != nil {
				return 0, ErrInvalid
			}
			roll = &n
		}
		v, e := normalizeStudent(StudentInput{AdmissionNo: record[0], NameEN: record[1], NameBN: record[2], DateOfBirth: record[3], RollNo: roll, Status: record[5], SessionID: sessionID, ClassID: classID, SectionID: sectionID})
		if e != nil {
			return 0, e
		}
		rows = append(rows, v)
	}
	if len(rows) == 0 {
		return 0, ErrInvalid
	}
	return s.repo.ImportStudents(ctx, a, rows)
}
