// Package calc is the examination result engine: grading, pass rules, GPA,
// ranking and the building blocks of term and annual results. It is pure (no
// database, no clock) and exact: marks and grade points are integer
// hundredths, derived values are rationals, and nothing is compared or
// rounded through binary floating point. The rules are documented in
// docs/exams/E1_RESULT_CALCULATION_RULES.md.
package calc

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

// ErrInvalid reports input the rules cannot grade: marks with more than two
// decimals, marks above full marks, an empty or malformed grading scheme.
var ErrInvalid = errors.New("invalid result input")

// Marks are hundredths of a mark: 16.83 is Marks(1683).
type Marks int64

// Points are hundredths of a grade point: 3.50 is Points(350).
type Points int64

// maxHundredths bounds parsed values well inside int64 arithmetic: products
// such as marks × 100 × full stay far below the int64 range.
const maxHundredths = 1_000_000_000

// MarksOf converts a stored two-decimal value (numeric(6,2) read as float64)
// to exact hundredths. It refuses negative, non-finite and over-precise values.
func MarksOf(v float64) (Marks, error) {
	h, err := hundredths(v)
	return Marks(h), err
}

// PointsOf converts a stored grade point such as 3.50 to exact hundredths.
func PointsOf(v float64) (Points, error) {
	h, err := hundredths(v)
	return Points(h), err
}

func hundredths(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, ErrInvalid
	}
	scaled := v * 100
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) >= 0.000001 || rounded > maxHundredths {
		return 0, ErrInvalid
	}
	return int64(rounded), nil
}

// Float returns the value for storage or JSON. Every Marks value is a
// two-decimal number, so the float's shortest representation is exact.
func (m Marks) Float() float64 { return float64(m) / 100 }

// Float returns the grade point for storage or JSON.
func (p Points) Float() float64 { return float64(p) / 100 }

// String formats with exactly two decimals ("16.83"), suitable for numeric
// columns.
func (m Marks) String() string { return twoDecimals(int64(m)) }

// String formats with exactly two decimals ("3.50").
func (p Points) String() string { return twoDecimals(int64(p)) }

func twoDecimals(h int64) string {
	sign := ""
	if h < 0 {
		sign, h = "-", -h
	}
	return fmt.Sprintf("%s%d.%02d", sign, h/100, h%100)
}

// Rounding is how a value is cut to two decimals (or whole marks).
type Rounding int

const (
	// HalfUp rounds exactly-half values up: 4.415 → 4.42.
	HalfUp Rounding = iota
	// Truncate drops the extra digits: 4.4166… → 4.41.
	Truncate
)

// roundRat cuts a non-negative rational to an integer count of units, where
// unit is the size of one step in hundredths (1 for 0.01, 100 for 1.00).
// The rational is expressed in hundredths.
func roundRat(r *big.Rat, unit int64, mode Rounding) int64 {
	if r.Sign() < 0 {
		panic("calc: negative value")
	}
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Mul(r.Denom(), big.NewInt(unit))
	if mode == HalfUp {
		// floor((2·num + den) / (2·den))
		num.Mul(num, big.NewInt(2)).Add(num, den)
		den.Mul(den, big.NewInt(2))
	}
	q := new(big.Int).Quo(num, den)
	return q.Int64() * unit
}

// RoundMarks cuts an exact value (in marks, not hundredths) to two decimals.
func RoundMarks(r *big.Rat, mode Rounding) Marks {
	h := new(big.Rat).Mul(r, big.NewRat(100, 1))
	return Marks(roundRat(h, 1, mode))
}

// WholeMarks rounds a two-decimal mark to whole marks (rounding point R3).
func WholeMarks(m Marks, mode Rounding) Marks {
	return Marks(roundRat(big.NewRat(int64(m), 1), 100, mode))
}

// Rat returns the exact value in marks.
func (m Marks) Rat() *big.Rat { return big.NewRat(int64(m), 100) }
