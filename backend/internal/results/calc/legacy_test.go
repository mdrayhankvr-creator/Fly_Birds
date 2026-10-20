package calc

import "math"

// The float engine as it was in internal/exams/service.go before E2, kept
// verbatim (apart from names) so the tests can reproduce its boundary defect
// and prove the exact engine agrees with it everywhere else.

type legacyBand struct {
	Grade    string
	MinMarks int
	MaxMarks int
	GPA      float64
}

type legacySubject struct {
	Marks     float64
	FullMarks float64
	PassMarks float64
	Fourth    bool
}

type legacyGrade struct {
	Name   string
	Point  float64
	Passed bool
}

func legacyGradeFor(marks, full, pass float64, bands []legacyBand) (legacyGrade, bool) {
	if !legacyFinite(marks) || !legacyFinite(full) || !legacyFinite(pass) || full <= 0 || marks > full || pass > full {
		return legacyGrade{}, false
	}
	pct := marks * 100 / full
	for _, b := range bands {
		if pct >= float64(b.MinMarks) && (pct < float64(b.MaxMarks+1) || b.MaxMarks == 100 && pct <= 100) {
			return legacyGrade{Name: b.Grade, Point: b.GPA, Passed: marks >= pass && b.GPA > 0}, true
		}
	}
	return legacyGrade{}, false
}

func legacyGPA(subjects []legacySubject, bands []legacyBand) (float64, bool, []legacyGrade, bool) {
	if len(subjects) == 0 {
		return 0, false, nil, false
	}
	required, optional := 0, 0
	sum, bonus, passed := 0.0, 0.0, true
	grades := make([]legacyGrade, 0, len(subjects))
	for _, s := range subjects {
		g, ok := legacyGradeFor(s.Marks, s.FullMarks, s.PassMarks, bands)
		if !ok {
			return 0, false, nil, false
		}
		grades = append(grades, g)
		if s.Fourth {
			optional++
			bonus = math.Max(0, g.Point-2)
			continue
		}
		required++
		sum += g.Point
		if !g.Passed {
			passed = false
		}
	}
	if required == 0 || optional > 1 {
		return 0, false, nil, false
	}
	if !passed {
		return 0, false, grades, true
	}
	return math.Round(math.Min(5, (sum+bonus)/float64(required))*100) / 100, true, grades, true
}

func legacyFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && math.Abs(v*100-math.Round(v*100)) < 0.000001
}

// bdDefault is the bd_default scheme from migration 0003, in both forms.
var bdDefaultLegacy = []legacyBand{{"A+", 80, 100, 5}, {"A", 70, 79, 4}, {"A-", 60, 69, 3.5}, {"B", 50, 59, 3}, {"C", 40, 49, 2}, {"D", 33, 39, 1}, {"F", 0, 32, 0}}

func bdDefault() Scheme {
	bands := make([]Band, 0, len(bdDefaultLegacy))
	for _, b := range bdDefaultLegacy {
		p, err := PointsOf(b.GPA)
		if err != nil {
			panic(err)
		}
		bands = append(bands, Band{Grade: b.Grade, Min: b.MinMarks, Max: b.MaxMarks, Point: p})
	}
	s, err := NewScheme(bands)
	if err != nil {
		panic(err)
	}
	return s
}
