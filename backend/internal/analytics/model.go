// Package analytics aggregates the existing attendance, examination, fee,
// student and staff records into school, class, teacher, student and guardian
// views. It never stores a copy of those records: every figure is computed
// from PostgreSQL on request, inside bounded date ranges and pages.
package analytics

import (
	"errors"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid analytics request")
	ErrNotFound  = errors.New("analytics record not found")
	ErrForbidden = errors.New("analytics access denied")
)

// MaxRangeDays bounds every dated query; MaxReportRows bounds a CSV report.
const (
	MaxRangeDays  = 400
	MaxReportRows = 20000
	MaxPageSize   = 100
)

// Dhaka is the school's calendar time zone.
var Dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// Scope says whose records a viewer may aggregate. School scope reads every
// class; teacher scope reads only classes (and, for subject figures, subjects)
// the viewer is assigned to in the selected session.
type Scope struct {
	Teacher bool
	UserID  string
	Finance bool // viewer may also see fee figures
}

type Filter struct {
	SessionID string `json:"session_id"`
	ClassID   string `json:"class_id,omitempty"`
	SectionID string `json:"section_id,omitempty"`
	ExamID    string `json:"exam_id,omitempty"`
	ExamType  string `json:"exam_type,omitempty"`
	From      string `json:"from"`
	To        string `json:"to"`
	Group     string `json:"group"`
}

type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	IsActive  bool   `json:"is_active"`
}
type Class struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"display_order"`
}
type Section struct {
	ID      string `json:"id"`
	ClassID string `json:"class_id"`
	Name    string `json:"name"`
}
type Exam struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ExamType  string `json:"exam_type"`
	ClassID   string `json:"class_id"`
	SectionID string `json:"section_id"`
}

// Options lists the filter choices the viewer may use.
type Options struct {
	Sessions []Session       `json:"sessions"`
	Session  *Session        `json:"session"`
	Classes  []Class         `json:"classes"`
	Sections []Section       `json:"sections"`
	Exams    []Exam          `json:"exams"`
	Modules  map[string]bool `json:"modules"`
	Today    string          `json:"today"`
	Settings Settings        `json:"settings"`
}

type Settings struct {
	AttendanceThreshold float64 `json:"attendance_threshold"`
	AttendanceWindow    int     `json:"attendance_window_days"`
	MinMarkedDays       int     `json:"min_marked_days"`
	GPAThreshold        float64 `json:"gpa_threshold"`
	ConsecutiveAbsences int     `json:"consecutive_absences"`
	InactivityDays      int     `json:"inactivity_days"`
	UpdatedAt           string  `json:"updated_at,omitempty"`
}

// Counts are attendance totals. Rate is (present + late) / marked, as a
// percentage with one decimal; it is null when nothing was marked.
type Counts struct {
	Marked  int64    `json:"marked"`
	Present int64    `json:"present"`
	Late    int64    `json:"late"`
	Absent  int64    `json:"absent"`
	Rate    *float64 `json:"rate"`
}
type TrendPoint struct {
	Period string `json:"period"`
	Counts
}
type GroupCounts struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Counts
}
type TeacherCounts struct {
	Marked  int64    `json:"marked"`
	Present int64    `json:"present"`
	Absent  int64    `json:"absent"`
	Leave   int64    `json:"leave"`
	Rate    *float64 `json:"rate"` // present / (present + absent)
}
type TeacherTrendPoint struct {
	Period string `json:"period"`
	TeacherCounts
}
type Attendance struct {
	Filter       Filter              `json:"filter"`
	Summary      Counts              `json:"summary"`
	Days         int64               `json:"days"`
	Students     int64               `json:"students"`
	Trend        []TrendPoint        `json:"trend"`
	GroupBy      string              `json:"group_by"`
	Groups       []GroupCounts       `json:"groups"`
	Teachers     *TeacherCounts      `json:"teachers,omitempty"`
	TeacherTrend []TeacherTrendPoint `json:"teacher_trend,omitempty"`
}

type ExamRow struct {
	ExamID      string   `json:"exam_id"`
	Name        string   `json:"name"`
	ExamType    string   `json:"exam_type"`
	ClassName   string   `json:"class_name"`
	SectionName string   `json:"section_name"`
	PublishedOn string   `json:"published_on"`
	Results     int64    `json:"results"`
	Passed      int64    `json:"passed"`
	PassRate    *float64 `json:"pass_rate"`
	AvgGPA      *float64 `json:"avg_gpa"`
}
type ResultSummary struct {
	Results  int64    `json:"results"`
	Passed   int64    `json:"passed"`
	Failed   int64    `json:"failed"`
	PassRate *float64 `json:"pass_rate"`
	AvgGPA   *float64 `json:"avg_gpa"`
	GoldenA  int64    `json:"gpa5"`
}
type GradeCount struct {
	Grade string  `json:"grade"`
	GPA   float64 `json:"gpa"`
	Count int64   `json:"count"`
}
type SubjectRow struct {
	SubjectID  string   `json:"subject_id"`
	NameEN     string   `json:"name_en"`
	NameBN     string   `json:"name_bn"`
	Code       string   `json:"code"`
	Entries    int64    `json:"entries"`
	Passed     int64    `json:"passed"`
	PassRate   *float64 `json:"pass_rate"`
	AvgPercent *float64 `json:"avg_percent"`
}
type GroupResult struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Results  int64    `json:"results"`
	Passed   int64    `json:"passed"`
	PassRate *float64 `json:"pass_rate"`
	AvgGPA   *float64 `json:"avg_gpa"`
}
type Exams struct {
	Filter       Filter        `json:"filter"`
	Summary      ResultSummary `json:"summary"`
	Exams        []ExamRow     `json:"exams"`
	Distribution []GradeCount  `json:"distribution"`
	Subjects     []SubjectRow  `json:"subjects"`
	GroupBy      string        `json:"group_by"`
	Groups       []GroupResult `json:"groups"`
}

type FeeSummary struct {
	Billed          float64  `json:"billed"`
	Discounts       float64  `json:"discounts"`
	NetBilled       float64  `json:"net_billed"`
	PaidAgainst     float64  `json:"paid_against_billed"`
	CollectionRate  *float64 `json:"collection_rate"`
	Collected       float64  `json:"collected"`
	Outstanding     float64  `json:"outstanding"`
	Overdue         float64  `json:"overdue"`
	OverdueStudents int64    `json:"overdue_students"`
	Income          float64  `json:"income"`
	Expense         float64  `json:"expense"`
	Net             float64  `json:"net"`
}
type FeeMonth struct {
	Month     string  `json:"month"`
	Billed    float64 `json:"billed"`
	Collected float64 `json:"collected"`
	Income    float64 `json:"income"`
	Expense   float64 `json:"expense"`
}
type MethodRow struct {
	Method   string  `json:"method"`
	Amount   float64 `json:"amount"`
	Receipts int64   `json:"receipts"`
}
type ClassFees struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	NetBilled   float64  `json:"net_billed"`
	Paid        float64  `json:"paid"`
	Outstanding float64  `json:"outstanding"`
	Rate        *float64 `json:"rate"`
}
type AgingBucket struct {
	Bucket   string  `json:"bucket"`
	Amount   float64 `json:"amount"`
	Students int64   `json:"students"`
}
type Overdue struct {
	StudentID   string  `json:"student_id"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	AdmissionNo string  `json:"admission_no"`
	ClassName   string  `json:"class_name"`
	SectionName string  `json:"section_name"`
	Overdue     float64 `json:"overdue"`
	OldestDue   string  `json:"oldest_due"`
}
type CategoryAmount struct {
	EntryType string  `json:"entry_type"`
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
}
type Fees struct {
	Filter     Filter           `json:"filter"`
	Summary    FeeSummary       `json:"summary"`
	Months     []FeeMonth       `json:"months"`
	Methods    []MethodRow      `json:"methods"`
	Classes    []ClassFees      `json:"classes"`
	Aging      []AgingBucket    `json:"aging"`
	TopOverdue []Overdue        `json:"top_overdue"`
	Ledger     []CategoryAmount `json:"ledger"`
}

type SessionRow struct {
	Session
	ActiveStudents int64    `json:"active_students"`
	Marked         int64    `json:"attendance_marked"`
	AttendanceRate *float64 `json:"attendance_rate"`
	Exams          int64    `json:"exams"`
	Results        int64    `json:"results"`
	PassRate       *float64 `json:"pass_rate"`
	AvgGPA         *float64 `json:"avg_gpa"`
	NetBilled      *float64 `json:"net_billed,omitempty"`
	Collected      *float64 `json:"collected,omitempty"`
	CollectionRate *float64 `json:"collection_rate,omitempty"`
}

type ClassCount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Students int64  `json:"students"`
}
type Overview struct {
	Session         *Session         `json:"session"`
	ActiveStudents  int64            `json:"active_students"`
	StudentsByState map[string]int64 `json:"students_by_status"`
	ByClass         []ClassCount     `json:"by_class"`
	Teachers        int64            `json:"teachers"`
	NonTeaching     int64            `json:"non_teaching_staff"`
	Guardians       int64            `json:"guardians"`
	Today           string           `json:"today"`
	TodayStudents   *Counts          `json:"today_students"`
	TodayTeachers   *TeacherCounts   `json:"today_teachers"`
	Last30          *Counts          `json:"last_30_days"`
	Results         *ResultSummary   `json:"results"`
	Risks           *RiskSummary     `json:"risks"`
	Fees            *FeeSummary      `json:"fees,omitempty"`
}

// Risk reasons. At-risk: low_attendance, low_gpa, failed_exam. Dropout:
// consecutive_absences, no_recent_presence.
type Risk struct {
	StudentID      string   `json:"student_id"`
	NameEN         string   `json:"name_en"`
	NameBN         string   `json:"name_bn"`
	AdmissionNo    string   `json:"admission_no"`
	RollNo         *int     `json:"roll_no"`
	ClassName      string   `json:"class_name"`
	SectionName    string   `json:"section_name"`
	Level          string   `json:"level"` // dropout or at_risk
	Reasons        []string `json:"reasons"`
	Marked         int64    `json:"marked"`
	AttendanceRate *float64 `json:"attendance_rate"`
	AbsentStreak   int64    `json:"absent_streak"`
	LastPresent    *string  `json:"last_present"`
	LatestExam     *string  `json:"latest_exam"`
	LatestGPA      *float64 `json:"latest_gpa"`
	LatestPassed   *bool    `json:"latest_passed"`
}
type RiskSummary struct {
	AtRisk   int64            `json:"at_risk"`
	Dropout  int64            `json:"dropout"`
	Students int64            `json:"students"`
	Reasons  map[string]int64 `json:"reasons"`
}
type RiskPage struct {
	Filter   Filter      `json:"filter"`
	Summary  RiskSummary `json:"summary"`
	Settings Settings    `json:"settings"`
	Items    []Risk      `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type StudentInfo struct {
	ID          string `json:"id"`
	NameEN      string `json:"name_en"`
	NameBN      string `json:"name_bn"`
	AdmissionNo string `json:"admission_no"`
	RollNo      *int   `json:"roll_no"`
	Status      string `json:"status"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	ClassID     string `json:"class_id"`
	ClassName   string `json:"class_name"`
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
}
type DayStatus struct {
	Date   string `json:"date"`
	Status string `json:"status"`
}
type StudentAttendance struct {
	Session Counts       `json:"session"`
	Last30  Counts       `json:"last_30_days"`
	Months  []TrendPoint `json:"months"`
	Recent  []DayStatus  `json:"recent"`
}
type StudentResult struct {
	ExamID          string   `json:"exam_id"`
	Name            string   `json:"name"`
	ExamType        string   `json:"exam_type"`
	PublishedOn     string   `json:"published_on"`
	TotalMarks      float64  `json:"total_marks"`
	FullMarks       float64  `json:"full_marks"`
	Percent         *float64 `json:"percent"`
	GPA             float64  `json:"gpa"`
	Grade           string   `json:"grade"`
	Passed          bool     `json:"passed"`
	ClassPosition   int      `json:"class_position"`
	SectionPosition int      `json:"section_position"`
}
type StudentSubject struct {
	NameEN     string   `json:"name_en"`
	NameBN     string   `json:"name_bn"`
	Marks      float64  `json:"marks"`
	FullMarks  float64  `json:"full_marks"`
	Percent    *float64 `json:"percent"`
	Grade      *string  `json:"grade"`
	GradePoint *float64 `json:"grade_point"`
	Passed     *bool    `json:"passed"`
}
type StudentFees struct {
	Outstanding float64 `json:"outstanding"`
	Overdue     float64 `json:"overdue"`
	PaidSession float64 `json:"paid_session"`
	NextDue     *string `json:"next_due"`
}
type StudentSummary struct {
	Student        StudentInfo        `json:"student"`
	Today          string             `json:"today"`
	Attendance     *StudentAttendance `json:"attendance"`
	Results        []StudentResult    `json:"results"`
	LatestSubjects []StudentSubject   `json:"latest_subjects"`
	Fees           *StudentFees       `json:"fees"`
	Alert          *Risk              `json:"alert"`
	Settings       Settings           `json:"settings"`
}

// ReportPage is one page of a tabular report; CSV exports use the same rows.
type ReportPage struct {
	Report   string     `json:"report"`
	Filter   Filter     `json:"filter"`
	Columns  []string   `json:"columns"`
	Rows     [][]string `json:"rows"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}
