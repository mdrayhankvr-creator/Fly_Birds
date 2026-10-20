package calc

import "sort"

// Band is one row of a grading scheme: a percentage range (whole percents,
// inclusive) and its grade point. The bd_default scheme has A+ 80–100 (5.00)
// down to F 0–32 (0.00).
type Band struct {
	Grade string
	Min   int
	Max   int
	Point Points
}

// Scheme is a grading scheme ordered from the highest band down.
type Scheme struct{ bands []Band }

// NewScheme orders the bands by their lower edge, highest first, and refuses
// an empty scheme or a band outside 0–100 or 0.00–5.00. A gap between bands
// is allowed (as today); a percentage that falls in it cannot be graded.
func NewScheme(bands []Band) (Scheme, error) {
	if len(bands) == 0 {
		return Scheme{}, ErrInvalid
	}
	out := append([]Band(nil), bands...)
	for _, b := range out {
		if b.Min < 0 || b.Max > 100 || b.Min > b.Max || b.Point < 0 || b.Point > 500 {
			return Scheme{}, ErrInvalid
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Min > out[j].Min })
	return Scheme{bands: out}, nil
}

// Bands returns a copy of the ordered bands.
func (s Scheme) Bands() []Band { return append([]Band(nil), s.bands...) }

// SubjectGrade is the graded outcome of one subject.
type SubjectGrade struct {
	Grade  string
	Point  Points
	Passed bool
}

// Band finds the band of marks out of full without computing a percentage:
// marks reach a lower edge L when marks × 100 ≥ L × full, which compares
// integers exactly. A band's upper edge M includes everything below M + 1
// percent (79.99 % is in 70–79), and the band ending at 100 includes 100 %.
func (s Scheme) Band(marks, full Marks) (Band, error) {
	if full <= 0 || marks < 0 || marks > full {
		return Band{}, ErrInvalid
	}
	scaled := int64(marks) * 100
	for _, b := range s.bands {
		atLeastMin := scaled >= int64(b.Min)*int64(full)
		belowNext := scaled < int64(b.Max+1)*int64(full)
		if atLeastMin && (belowNext || b.Max == 100) {
			return b, nil
		}
	}
	return Band{}, ErrInvalid
}

// GradeSubject grades one subject. It passes when the marks reach the pass
// mark and the band's grade point is above zero (the F band always fails).
func GradeSubject(marks, full, pass Marks, s Scheme) (SubjectGrade, error) {
	if pass < 0 || pass > full {
		return SubjectGrade{}, ErrInvalid
	}
	b, err := s.Band(marks, full)
	if err != nil {
		return SubjectGrade{}, err
	}
	return SubjectGrade{Grade: b.Grade, Point: b.Point, Passed: marks >= pass && b.Point > 0}, nil
}
