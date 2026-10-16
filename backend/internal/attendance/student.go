package attendance

import (
	"context"
	"strings"
	"unicode/utf8"
)

type StudentSection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
	SessionID string `json:"session_id"`
}
type StudentRecord struct {
	ID             string `json:"id"`
	StudentID      string `json:"student_id"`
	SessionID      string `json:"session_id"`
	ClassID        string `json:"class_id"`
	SectionID      string `json:"section_id"`
	AttendanceDate string `json:"attendance_date"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	Note           string `json:"note"`
	NameEN         string `json:"name_en"`
	NameBN         string `json:"name_bn"`
	AdmissionNo    string `json:"admission_no"`
}
type StudentRosterItem struct {
	StudentID   string  `json:"student_id"`
	AdmissionNo string  `json:"admission_no"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	RollNo      *int    `json:"roll_no"`
	RecordID    *string `json:"record_id"`
	Status      *string `json:"status"`
	Source      *string `json:"source"`
	Note        *string `json:"note"`
}
type StudentSummary struct {
	StudentID   string `json:"student_id"`
	AdmissionNo string `json:"admission_no"`
	NameEN      string `json:"name_en"`
	NameBN      string `json:"name_bn"`
	Present     int64  `json:"present"`
	Absent      int64  `json:"absent"`
	Late        int64  `json:"late"`
	Total       int64  `json:"total"`
}
type StudentReport struct {
	Student StudentSummary  `json:"student"`
	Items   []StudentRecord `json:"items"`
}
type AbsenceEvent struct {
	ID             string `json:"id"`
	StudentID      string `json:"student_id"`
	AttendanceDate string `json:"attendance_date"`
	State          string `json:"state"`
	NameEN         string `json:"name_en"`
	GuardianCount  int64  `json:"guardian_count"`
}
type StudentQuery struct {
	SessionID string
	SectionID string
	Date      string
	Month     string
	Search    string
	Page      int
	PageSize  int
}
type StudentInput struct {
	StudentID      string `json:"student_id"`
	AttendanceDate string `json:"attendance_date"`
	Status         string `json:"status"`
	Note           string `json:"note"`
}
type StudentCorrection struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}
type ScanInput struct {
	Payload        string `json:"payload"`
	AttendanceDate string `json:"attendance_date"`
}
type StudentRepository interface {
	Sections(context.Context, string, string) ([]StudentSection, error)
	StudentRoster(context.Context, string, StudentQuery) (Page[StudentRosterItem], error)
	RecordStudent(context.Context, string, StudentInput, string) (StudentRecord, error)
	CorrectStudent(context.Context, string, string, StudentCorrection) (StudentRecord, error)
	StudentMonthly(context.Context, string, StudentQuery) (Page[StudentSummary], error)
	StudentReport(context.Context, string, string, string, string) (StudentReport, error)
	AbsenceEvents(context.Context, string, StudentQuery) (Page[AbsenceEvent], error)
}
type StudentService struct{ repo StudentRepository }

func NewStudentService(repo StudentRepository) *StudentService { return &StudentService{repo: repo} }
func validStudentStatus(v string) bool                         { return v == "present" || v == "absent" || v == "late" }
func validStudentQuery(q StudentQuery) bool {
	return q.Page >= 1 && q.Page <= 100000 && q.PageSize >= 1 && q.PageSize <= 100 && utf8.ValidString(q.Search) && len(q.Search) <= 100
}
func (s *StudentService) Sections(ctx context.Context, actor, sessionID string) ([]StudentSection, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(sessionID) {
		return nil, ErrInvalid
	}
	return s.repo.Sections(ctx, actor, sessionID)
}
func (s *StudentService) Roster(ctx context.Context, actor string, q StudentQuery) (Page[StudentRosterItem], error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(q.SessionID) || !uuid.MatchString(q.SectionID) || !validDate(q.Date) || !validStudentQuery(q) {
		return Page[StudentRosterItem]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.StudentRoster(ctx, actor, q)
}
func (s *StudentService) Record(ctx context.Context, actor string, in StudentInput) (StudentRecord, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(in.StudentID) || !validDate(in.AttendanceDate) || !validStudentStatus(in.Status) || !validNote(in.Note) {
		return StudentRecord{}, ErrInvalid
	}
	in.Note = strings.TrimSpace(in.Note)
	return s.repo.RecordStudent(ctx, actor, in, "manual")
}
func (s *StudentService) Scan(ctx context.Context, actor string, in ScanInput) (StudentRecord, error) {
	// The M3 card QR carries a student UUID. It is an identifier, never authorization.
	if !uuid.MatchString(actor) || !strings.HasPrefix(in.Payload, "FB-STUDENT:") || !uuid.MatchString(strings.TrimPrefix(in.Payload, "FB-STUDENT:")) || !validDate(in.AttendanceDate) {
		return StudentRecord{}, ErrInvalid
	}
	return s.repo.RecordStudent(ctx, actor, StudentInput{StudentID: strings.TrimPrefix(in.Payload, "FB-STUDENT:"), AttendanceDate: in.AttendanceDate, Status: "present"}, "qr")
}
func (s *StudentService) Correct(ctx context.Context, actor, id string, in StudentCorrection) (StudentRecord, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || !validStudentStatus(in.Status) || !validNote(in.Note) {
		return StudentRecord{}, ErrInvalid
	}
	in.Note = strings.TrimSpace(in.Note)
	return s.repo.CorrectStudent(ctx, actor, id, in)
}
func (s *StudentService) Monthly(ctx context.Context, actor string, q StudentQuery) (Page[StudentSummary], error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(q.SessionID) || !uuid.MatchString(q.SectionID) || !monthPattern.MatchString(q.Month) || !validStudentQuery(q) {
		return Page[StudentSummary]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.StudentMonthly(ctx, actor, q)
}
func (s *StudentService) Report(ctx context.Context, actor, studentID, month, scope string) (StudentReport, error) {
	if !uuid.MatchString(actor) || (studentID != "" && !uuid.MatchString(studentID)) || !monthPattern.MatchString(month) || (scope != "self" && scope != "guardian") {
		return StudentReport{}, ErrInvalid
	}
	return s.repo.StudentReport(ctx, actor, studentID, month, scope)
}
func (s *StudentService) Alerts(ctx context.Context, actor string, q StudentQuery) (Page[AbsenceEvent], error) {
	if !uuid.MatchString(actor) || !validStudentQuery(q) || (q.Date != "" && !validDate(q.Date)) {
		return Page[AbsenceEvent]{}, ErrInvalid
	}
	return s.repo.AbsenceEvents(ctx, actor, q)
}
