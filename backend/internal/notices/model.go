package notices

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInvalid   = errors.New("invalid notice or event input")
	ErrNotFound  = errors.New("notice or event not found")
	ErrConflict  = errors.New("notice or event state conflict")
	ErrForbidden = errors.New("notice or event access denied")
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// AudienceRoles are the roles a notice or event can address. Administrators
// always see everything and are not listed.
var AudienceRoles = []string{"head_teacher", "teacher", "student", "guardian", "accountant"}

type Target struct {
	ClassID     string  `json:"class_id"`
	SectionID   *string `json:"section_id"`
	ClassName   string  `json:"class_name,omitempty"`
	SectionName string  `json:"section_name,omitempty"`
}
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}
type Notice struct {
	ID            string       `json:"id"`
	TitleEN       string       `json:"title_en"`
	TitleBN       string       `json:"title_bn"`
	BodyEN        string       `json:"body_en"`
	BodyBN        string       `json:"body_bn"`
	Priority      string       `json:"priority"`
	AudienceRoles []string     `json:"audience_roles" gorm:"-"`
	RolesJSON     string       `json:"-"`
	Status        string       `json:"status"`
	PublishAt     *string      `json:"publish_at"`
	ExpiresAt     *string      `json:"expires_at"`
	Live          bool         `json:"live"`
	AuthorName    string       `json:"author_name"`
	UpdatedAt     string       `json:"updated_at"`
	Targets       []Target     `json:"targets" gorm:"-"`
	Attachments   []Attachment `json:"attachments" gorm:"-"`
}
type NoticeInput struct {
	TitleEN       string   `json:"title_en"`
	TitleBN       string   `json:"title_bn"`
	BodyEN        string   `json:"body_en"`
	BodyBN        string   `json:"body_bn"`
	Priority      string   `json:"priority"`
	AudienceRoles []string `json:"audience_roles"`
	Targets       []Target `json:"targets"`
	ExpiresAt     *string  `json:"expires_at"`
	FileIDs       []string `json:"file_ids"`
}
type PublishInput struct {
	PublishAt *string `json:"publish_at"`
}
type Event struct {
	ID            string   `json:"id"`
	TitleEN       string   `json:"title_en"`
	TitleBN       string   `json:"title_bn"`
	DescriptionEN string   `json:"description_en"`
	DescriptionBN string   `json:"description_bn"`
	Kind          string   `json:"kind"`
	StartsOn      string   `json:"starts_on"`
	EndsOn        string   `json:"ends_on"`
	StartTime     *string  `json:"start_time"`
	EndTime       *string  `json:"end_time"`
	Location      string   `json:"location"`
	AudienceRoles []string `json:"audience_roles" gorm:"-"`
	RolesJSON     string   `json:"-"`
	Status        string   `json:"status"`
	Targets       []Target `json:"targets" gorm:"-"`
}
type EventInput struct {
	TitleEN       string   `json:"title_en"`
	TitleBN       string   `json:"title_bn"`
	DescriptionEN string   `json:"description_en"`
	DescriptionBN string   `json:"description_bn"`
	Kind          string   `json:"kind"`
	StartsOn      string   `json:"starts_on"`
	EndsOn        string   `json:"ends_on"`
	StartTime     *string  `json:"start_time"`
	EndTime       *string  `json:"end_time"`
	Location      string   `json:"location"`
	AudienceRoles []string `json:"audience_roles"`
	Targets       []Target `json:"targets"`
}
type Query struct {
	Status   string
	Search   string
	Priority string
	From     string
	To       string
	Page     int
	PageSize int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// AudiencePredicate returns SQL that is true when the user aliased "u" may see
// the row aliased entity, whose class/section targets live in targetTable
// keyed by fk. It is shared by the feeds and by notification dispatch.
func AudiencePredicate(entity, targetTable, fk string) string {
	return fmt.Sprintf(`(u.role = 'admin' OR (u.role = ANY(%[1]s.audience_roles) AND (
 u.role = 'accountant'
 OR NOT EXISTS (SELECT 1 FROM %[2]s t WHERE t.%[3]s = %[1]s.id)
 OR (u.role = 'student' AND EXISTS (SELECT 1 FROM %[2]s t JOIN students s ON s.class_id = t.class_id AND (t.section_id IS NULL OR t.section_id = s.section_id) WHERE t.%[3]s = %[1]s.id AND s.user_id = u.id))
 OR (u.role = 'guardian' AND EXISTS (SELECT 1 FROM %[2]s t JOIN students s ON s.class_id = t.class_id AND (t.section_id IS NULL OR t.section_id = s.section_id) JOIN student_guardians sg ON sg.student_id = s.id JOIN guardians g ON g.id = sg.guardian_id WHERE t.%[3]s = %[1]s.id AND g.user_id = u.id))
 OR (u.role IN ('teacher', 'head_teacher') AND EXISTS (SELECT 1 FROM %[2]s t JOIN teacher_assignments a ON a.class_id = t.class_id JOIN employees e ON e.id = a.employee_id WHERE t.%[3]s = %[1]s.id AND e.user_id = u.id AND e.is_active))
)))`, entity, targetTable, fk)
}

// pgTextArray renders validated enum values as a PostgreSQL array literal.
func pgTextArray(values []string) string { return "{" + strings.Join(values, ",") + "}" }
