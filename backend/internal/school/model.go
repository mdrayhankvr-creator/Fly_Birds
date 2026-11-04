package school

import "time"

const SingleSchoolID = 1

// SchoolSettings is one deployment's persisted profile. The repository uses
// SingleSchoolID so a future tenant-aware repository can replace it cleanly.
type SchoolSettings struct {
	ID                int    `gorm:"primaryKey" json:"-"`
	NameEN            string `json:"name_en"`
	NameBN            string `json:"name_bn"`
	LogoURL           string `json:"logo_url"`
	Address           string `json:"address"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	PreferredLanguage string `json:"preferred_language"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	DashboardTheme    string `json:"dashboard_theme"`
	GradingSchemeID   string `json:"grading_scheme_id"`
	TaglineEN         string `json:"tagline_en"`
	TaglineBN         string `json:"tagline_bn"`
	// School details (migration 1759), shown on the public page and read by
	// documents such as the marksheet. Address above is the English address.
	EIIN              string    `json:"eiin"`
	FoundedYear       *int      `json:"founded_year"`
	AddressBN         string    `gorm:"column:address_bn" json:"address_bn"`
	MapURL            string    `json:"map_url"`
	HeadTeacherNameEN string    `gorm:"column:head_teacher_name_en" json:"head_teacher_name_en"`
	HeadTeacherNameBN string    `gorm:"column:head_teacher_name_bn" json:"head_teacher_name_bn"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (SchoolSettings) TableName() string { return "school_settings" }

type SettingsInput struct {
	NameEN            string `json:"name_en"`
	NameBN            string `json:"name_bn"`
	LogoURL           string `json:"logo_url"`
	Address           string `json:"address"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	PreferredLanguage string `json:"preferred_language"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	DashboardTheme    string `json:"dashboard_theme"`
	GradingSchemeID   string `json:"grading_scheme_id"`
	// Taglines are optional so clients that predate them keep the saved text.
	TaglineEN *string `json:"tagline_en,omitempty"`
	TaglineBN *string `json:"tagline_bn,omitempty"`
	// School details are optional too: an omitted field keeps its value.
	// FoundedYear 0 clears the year.
	EIIN              *string `json:"eiin,omitempty"`
	FoundedYear       *int    `json:"founded_year,omitempty"`
	AddressBN         *string `json:"address_bn,omitempty"`
	MapURL            *string `json:"map_url,omitempty"`
	HeadTeacherNameEN *string `json:"head_teacher_name_en,omitempty"`
	HeadTeacherNameBN *string `json:"head_teacher_name_bn,omitempty"`
}

type Branding struct {
	NameEN            string `json:"name_en"`
	NameBN            string `json:"name_bn"`
	LogoURL           string `json:"logo_url"`
	PreferredLanguage string `json:"preferred_language"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	DashboardTheme    string `json:"dashboard_theme"`
	TaglineEN         string `json:"tagline_en"`
	TaglineBN         string `json:"tagline_bn"`
}

func (s SchoolSettings) Branding() Branding {
	return Branding{s.NameEN, s.NameBN, s.LogoURL, s.PreferredLanguage, s.PrimaryColor, s.SecondaryColor, s.DashboardTheme, s.TaglineEN, s.TaglineBN}
}

type GradeBand struct {
	SchemeID string  `gorm:"column:scheme_id;primaryKey" json:"-"`
	Grade    string  `gorm:"primaryKey" json:"grade"`
	MinMarks int     `json:"min_marks"`
	MaxMarks int     `json:"max_marks"`
	GPA      float64 `json:"gpa"`
}

func (GradeBand) TableName() string { return "grading_bands" }

type Module struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	IsCore      bool     `json:"is_core"`
	Enabled     bool     `json:"enabled"`
	DependsOn   []string `json:"depends_on" gorm:"-"`
	Dependents  []string `json:"dependents" gorm:"-"`
	DependsJSON string   `json:"-"`
	UpdatedAt   string   `json:"updated_at"`
	// Changed lists every module a cascaded update switched, in order.
	Changed []string `json:"changed,omitempty" gorm:"-"`
}
type ModulePage struct {
	Items    []Module `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}
