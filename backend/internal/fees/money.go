package fees

import (
	"fmt"
	"math"
)

// maxCents caps any single amount at 10 crore taka, well inside numeric(12,2).
const maxCents = int64(10_000_000_000)

// ToCents converts a JSON amount to integer poisha, rejecting negative,
// non-finite, oversized and sub-poisha values.
func ToCents(v float64) (int64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false
	}
	scaled := v * 100
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > 1e-6 || rounded > float64(maxCents) {
		return 0, false
	}
	return int64(rounded), true
}

// Decimal renders poisha as a PostgreSQL numeric literal.
func Decimal(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// PercentOf returns the rounded poisha share of amount for a percentage with
// two decimals, so concessions never produce fractional poisha.
func PercentOf(amount int64, percent float64) int64 {
	basis := int64(math.Round(percent * 100))
	return (amount*basis + 5000) / 10000
}

type Outstanding struct {
	ID      string
	Balance int64
}
type Split struct {
	ID     string
	Amount int64
}

// Allocate settles the oldest outstanding dues first. The payment must be
// positive and must not exceed the combined balance.
func Allocate(amount int64, dues []Outstanding) ([]Split, error) {
	if amount <= 0 {
		return nil, ErrInvalid
	}
	var available int64
	for _, d := range dues {
		if d.Balance > 0 {
			available += d.Balance
		}
	}
	if amount > available {
		return nil, ErrOverpayment
	}
	out := []Split{}
	remaining := amount
	for _, d := range dues {
		if remaining == 0 {
			break
		}
		if d.Balance <= 0 {
			continue
		}
		part := min(d.Balance, remaining)
		out = append(out, Split{ID: d.ID, Amount: part})
		remaining -= part
	}
	return out, nil
}
