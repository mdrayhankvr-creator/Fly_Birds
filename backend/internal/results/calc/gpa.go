package calc

import "math/big"

// Subject is one subject of an exam result as the current production engine
// sees it: marks out of full marks with a pass mark, optionally the fourth
// (optional) subject.
type Subject struct {
	Marks    Marks
	Full     Marks
	Pass     Marks
	Optional bool
}

// Policy holds the rounding choices that are school policy (rounding point R4
// in the calculation rules). The zero value is today's behaviour.
type Policy struct {
	GPA Rounding
}

// Result is an exam result for one student.
type Result struct {
	GPA    Points
	Passed bool
	Grades []SubjectGrade // in the order of the subjects given
	Total  Marks          // all subjects, the optional one included
	Full   Marks
}

// fourthSubjectBase is the grade point an optional subject must exceed to
// add to the GPA: the bonus is max(0, point − 2.00).
const fourthSubjectBase Points = 200

// maxGPA caps the GPA at 5.00.
const maxGPA Points = 500

// Calculate applies the production rules:
//   - every subject is graded with GradeSubject;
//   - a failed required subject gives GPA 0.00 and a failed result;
//   - the optional subject (at most one) adds max(0, point − 2.00) and never
//     fails the student;
//   - GPA = min(5.00, (required points + bonus) / required count), rounded to
//     two decimals by the policy.
func Calculate(subjects []Subject, s Scheme, p Policy) (Result, error) {
	grades := make([]SubjectGrade, 0, len(subjects))
	for _, sub := range subjects {
		g, err := GradeSubject(sub.Marks, sub.Full, sub.Pass, s)
		if err != nil {
			return Result{}, err
		}
		grades = append(grades, g)
	}
	return aggregate(subjects, grades, p)
}

// aggregate turns graded subjects into a result (totals, pass, GPA).
func aggregate(subjects []Subject, grades []SubjectGrade, p Policy) (Result, error) {
	if len(subjects) == 0 || len(grades) != len(subjects) {
		return Result{}, ErrInvalid
	}
	res := Result{Grades: grades}
	required, optional := int64(0), 0
	var sum, bonus Points
	passed := true
	for i, sub := range subjects {
		g := grades[i]
		res.Total += sub.Marks
		res.Full += sub.Full
		if sub.Optional {
			optional++
			bonus = max(0, g.Point-fourthSubjectBase)
			continue
		}
		required++
		sum += g.Point
		if !g.Passed {
			passed = false
		}
	}
	if required == 0 || optional > 1 {
		return Result{}, ErrInvalid
	}
	if !passed {
		return res, nil
	}
	gpa := big.NewRat(int64(sum+bonus), required) // in hundredths of a point
	if gpa.Cmp(big.NewRat(int64(maxGPA), 1)) > 0 {
		gpa.SetInt64(int64(maxGPA))
	}
	res.GPA = Points(roundRat(gpa, 1, p.GPA))
	res.Passed = true
	return res, nil
}
