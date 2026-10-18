package diary

import (
	"errors"
	"regexp"
)

var (
	ErrInvalid   = errors.New("invalid diary input")
	ErrNotFound  = errors.New("diary not found")
	ErrConflict  = errors.New("diary state conflict")
	ErrForbidden = errors.New("diary access denied")
	ErrClosed    = errors.New("homework submission is closed")
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

// Assignment is one section and subject a teacher may write for, with the
// status of that day's diary.
type Assignment struct {
	SessionID     string  `json:"session_id"`
	SessionName   string  `json:"session_name"`
	ClassID       string  `json:"class_id"`
	ClassName     string  `json:"class_name"`
	SectionID     string  `json:"section_id"`
	SectionName   string  `json:"section_name"`
	SubjectID     string  `json:"subject_id"`
	SubjectNameEN string  `json:"subject_name_en"`
	SubjectNameBN string  `json:"subject_name_bn"`
	DiaryID       *string `json:"diary_id"`
	DiaryStatus   *string `json:"diary_status"`
}

type Diary struct {
	ID                string       `json:"id"`
	SessionID         string       `json:"session_id"`
	ClassID           string       `json:"class_id"`
	SectionID         string       `json:"section_id"`
	SubjectID         string       `json:"subject_id"`
	ClassName         string       `json:"class_name"`
	SectionName       string       `json:"section_name"`
	SubjectNameEN     string       `json:"subject_name_en"`
	SubjectNameBN     string       `json:"subject_name_bn"`
	TeacherName       string       `json:"teacher_name"`
	DiaryDate         string       `json:"diary_date"`
	LessonTopics      string       `json:"lesson_topics"`
	ClassNotes        string       `json:"class_notes"`
	Homework          string       `json:"homework"`
	Instructions      string       `json:"instructions"`
	Status            string       `json:"status"`
	PublishedAt       *string      `json:"published_at"`
	SubmissionEnabled bool         `json:"submission_enabled"`
	SubmissionDueAt   *string      `json:"submission_due_at"`
	SubmissionOpen    bool         `json:"submission_open"`
	SubmissionCount   int64        `json:"submission_count"`
	MySubmission      *string      `json:"my_submission_status"`
	Attachments       []Attachment `json:"attachments" gorm:"-"`
}

type DiaryInput struct {
	SectionID         string   `json:"section_id"`
	SubjectID         string   `json:"subject_id"`
	DiaryDate         string   `json:"diary_date"`
	LessonTopics      string   `json:"lesson_topics"`
	ClassNotes        string   `json:"class_notes"`
	Homework          string   `json:"homework"`
	Instructions      string   `json:"instructions"`
	SubmissionEnabled bool     `json:"submission_enabled"`
	SubmissionDueAt   *string  `json:"submission_due_at"`
	FileIDs           []string `json:"file_ids"`
}

type Submission struct {
	ID            string       `json:"id"`
	DiaryID       string       `json:"diary_id"`
	StudentID     string       `json:"student_id"`
	StudentName   string       `json:"student_name"`
	StudentNameBN string       `json:"student_name_bn"`
	AdmissionNo   string       `json:"admission_no"`
	RollNo        *int         `json:"roll_no"`
	AnswerText    string       `json:"answer_text"`
	Status        string       `json:"status"`
	ReviewNote    string       `json:"review_note"`
	SubmittedAt   *string      `json:"submitted_at"`
	ReviewedAt    *string      `json:"reviewed_at"`
	Files         []Attachment `json:"files" gorm:"-"`
}

type SubmissionInput struct {
	AnswerText string   `json:"answer_text"`
	FileIDs    []string `json:"file_ids"`
}

type Settings struct {
	DailyRequired bool   `json:"daily_required"`
	ReminderTime  string `json:"reminder_time"`
}

// ComplianceRow reports whether a section and subject has today's diary.
type ComplianceRow struct {
	ClassName     string  `json:"class_name"`
	SectionName   string  `json:"section_name"`
	SubjectNameEN string  `json:"subject_name_en"`
	SubjectNameBN string  `json:"subject_name_bn"`
	Teachers      string  `json:"teachers"`
	DiaryStatus   *string `json:"diary_status"`
}

type Query struct {
	From      string
	To        string
	SubjectID string
	SectionID string
	Status    string
	Search    string
	Page      int
	PageSize  int
}

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// Viewer identifies who is reading. StudentID is the child for guardians.
type Viewer struct {
	UserID    string
	Role      string
	Oversight bool
	StudentID string
}
