package analytics

import (
	"math"
	"regexp"
	"sort"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validID(v string) bool { return v == "" || uuidPattern.MatchString(v) }

var examTypes = map[string]bool{"": true, "class_test": true, "half_yearly": true, "annual": true, "other": true}

// percent returns part/whole as a percentage rounded to one decimal, or nil
// when whole is zero, so "no data" is never shown as 0%.
func percent(part, whole float64) *float64 {
	if whole <= 0 {
		return nil
	}
	v := math.Round(part/whole*1000) / 10
	return &v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func ptr2(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := round2(*v)
	return &r
}

func (c *Counts) finish() { c.Rate = percent(float64(c.Present+c.Late), float64(c.Marked)) }
func (c *TeacherCounts) finish() {
	c.Rate = percent(float64(c.Present), float64(c.Present+c.Absent))
}

// Range validates a from..to date window, filling defaults: to defaults to
// today and from to `days` days before it. The window may not exceed
// MaxRangeDays or end after today.
func Range(from, to string, today time.Time, days int) (time.Time, time.Time, error) {
	end := today
	if to != "" {
		t, err := time.Parse(time.DateOnly, to)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalid
		}
		end = t
	}
	start := end.AddDate(0, 0, -(days - 1))
	if from != "" {
		f, err := time.Parse(time.DateOnly, from)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalid
		}
		start = f
	}
	if end.Before(start) || end.Sub(start) > (MaxRangeDays-1)*24*time.Hour {
		return time.Time{}, time.Time{}, ErrInvalid
	}
	return start, end, nil
}

// Grouping picks the trend bucket: an explicit day/week/month, or daily up to
// 62 days, weekly up to 190 days and monthly beyond.
func Grouping(group string, from, to time.Time) (string, error) {
	switch group {
	case "day", "week", "month":
		if group == "day" && to.Sub(from) > 92*24*time.Hour {
			return "", ErrInvalid // keeps a daily series to at most 93 points
		}
		return group, nil
	case "":
		days := int(to.Sub(from).Hours()/24) + 1
		if days <= 62 {
			return "day", nil
		}
		if days <= 190 {
			return "week", nil
		}
		return "month", nil
	}
	return "", ErrInvalid
}

// Band is one grading band of the school's configured scheme.
type Band struct {
	Grade string
	GPA   float64
}

// GradeFor maps an overall GPA to a letter using the school's bands: a failed
// result takes the lowest band; otherwise the highest band whose GPA does not
// exceed the result's GPA.
func GradeFor(bands []Band, gpa float64, passed bool) string {
	if len(bands) == 0 {
		return ""
	}
	sorted := append([]Band(nil), bands...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].GPA > sorted[j].GPA })
	if !passed {
		return sorted[len(sorted)-1].Grade
	}
	for _, b := range sorted {
		if gpa+1e-9 >= b.GPA {
			return b.Grade
		}
	}
	return sorted[len(sorted)-1].Grade
}

// Distribution counts results per band, keeping every band (zero included) in
// descending GPA order so charts have a stable axis.
func Distribution(bands []Band, rows []gpaCount) []GradeCount {
	sorted := append([]Band(nil), bands...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].GPA > sorted[j].GPA })
	out := make([]GradeCount, len(sorted))
	index := map[string]int{}
	for i, b := range sorted {
		out[i] = GradeCount{Grade: b.Grade, GPA: b.GPA}
		index[b.Grade] = i
	}
	for _, r := range rows {
		if i, ok := index[GradeFor(sorted, r.GPA, r.Passed)]; ok {
			out[i].Count += r.Count
		}
	}
	return out
}

type gpaCount struct {
	GPA    float64
	Passed bool
	Count  int64
}

// DefaultSettings are the thresholds before an administrator changes them.
func DefaultSettings() Settings {
	return Settings{AttendanceThreshold: 75, AttendanceWindow: 30, MinMarkedDays: 5, GPAThreshold: 2, ConsecutiveAbsences: 5, InactivityDays: 21}
}

// Validate checks thresholds against the same limits as the database.
func (s Settings) Validate() error {
	if s.AttendanceThreshold < 1 || s.AttendanceThreshold > 100 || s.AttendanceWindow < 7 || s.AttendanceWindow > 120 ||
		s.MinMarkedDays < 1 || s.MinMarkedDays > 60 || s.MinMarkedDays > s.AttendanceWindow || s.GPAThreshold < 0 || s.GPAThreshold > 5 ||
		s.ConsecutiveAbsences < 2 || s.ConsecutiveAbsences > 60 || s.InactivityDays < 7 || s.InactivityDays > 120 ||
		math.IsNaN(s.AttendanceThreshold) || math.IsNaN(s.GPAThreshold) {
		return ErrInvalid
	}
	return nil
}

// studentSignals are the per-student measurements the alert rules read.
type studentSignals struct {
	StudentID     string
	NameEN        string
	NameBN        string
	AdmissionNo   string
	RollNo        *int
	ClassName     string
	ClassOrder    int
	SectionName   string
	Marked        int64
	Attended      int64
	AbsentStreak  int64
	LastPresent   *string
	SectionDays   int64 // distinct dates the section was marked in the inactivity window
	EnrolledEarly bool  // created before the inactivity window began
	LatestExam    *string
	LatestGPA     *float64
	LatestPassed  *bool
}

// Classify applies the alert rules. Dropout signals (an unbroken run of
// absences, or no presence while the section kept taking attendance) outrank
// at-risk signals (low attendance in the window, a failed latest exam, or a
// latest GPA below the threshold). It returns nil when nothing applies.
func Classify(s studentSignals, set Settings, attendance, exams bool, inactivityStart string) *Risk {
	reasons := []string{}
	dropout := false
	rate := percent(float64(s.Attended), float64(s.Marked))
	if attendance {
		if s.AbsentStreak >= int64(set.ConsecutiveAbsences) {
			reasons, dropout = append(reasons, "consecutive_absences"), true
		}
		if s.SectionDays >= int64(set.MinMarkedDays) && s.EnrolledEarly && (s.LastPresent == nil || *s.LastPresent < inactivityStart) {
			reasons, dropout = append(reasons, "no_recent_presence"), true
		}
		if s.Marked >= int64(set.MinMarkedDays) && rate != nil && *rate < set.AttendanceThreshold {
			reasons = append(reasons, "low_attendance")
		}
	}
	if exams && s.LatestPassed != nil {
		if !*s.LatestPassed {
			reasons = append(reasons, "failed_exam")
		} else if s.LatestGPA != nil && *s.LatestGPA < set.GPAThreshold {
			reasons = append(reasons, "low_gpa")
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	level := "at_risk"
	if dropout {
		level = "dropout"
	}
	return &Risk{StudentID: s.StudentID, NameEN: s.NameEN, NameBN: s.NameBN, AdmissionNo: s.AdmissionNo, RollNo: s.RollNo,
		ClassName: s.ClassName, SectionName: s.SectionName, Level: level, Reasons: reasons, Marked: s.Marked, AttendanceRate: rate,
		AbsentStreak: s.AbsentStreak, LastPresent: s.LastPresent, LatestExam: s.LatestExam, LatestGPA: ptr2(s.LatestGPA), LatestPassed: s.LatestPassed}
}
