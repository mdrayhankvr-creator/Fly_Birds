package school

import "time"

const SingleSchoolID = 1

// SchoolSettings is one deployment's persisted profile. The repository uses
// SingleSchoolID so a future tenant-aware repository can replace it cleanly.
type SchoolSettings struct {
	ID                int       `gorm:"primaryKey" json:"-"`
	NameEN            string    `json:"name_en"`
	NameBN            string    `json:"name_bn"`
	LogoURL           string    `json:"logo_url"`
	Address           string    `json:"address"`
	Phone             string    `json:"phone"`
	Email             string    `json:"email"`
	PreferredLanguage string    `json:"preferred_language"`
	PrimaryColor      string    `json:"primary_color"`
	SecondaryColor    string    `json:"secondary_color"`
	DashboardTheme    string    `json:"dashboard_theme"`
	GradingSchemeID   string    `json:"grading_scheme_id"`
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
}

type Branding struct {
	NameEN            string `json:"name_en"`
	NameBN            string `json:"name_bn"`
	LogoURL           string `json:"logo_url"`
	PreferredLanguage string `json:"preferred_language"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	DashboardTheme    string `json:"dashboard_theme"`
}

func (s SchoolSettings) Branding() Branding {
	return Branding{s.NameEN, s.NameBN, s.LogoURL, s.PreferredLanguage, s.PrimaryColor, s.SecondaryColor, s.DashboardTheme}
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
	Key     string `json:"key"`
	Name    string `json:"name"`
	IsCore  bool   `json:"is_core"`
	Enabled bool   `json:"enabled"`
}
type ModulePage struct {
	Items    []Module `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}
