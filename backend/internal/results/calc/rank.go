package calc

import "sort"

// RankMode is how tied students share positions.
type RankMode int

const (
	// Competition ranking: 1, 1, 3 (today's behaviour, SQL rank()).
	Competition RankMode = iota
	// Dense ranking: 1, 1, 2.
	Dense
)

// Ranked is one student's place in a merit list. Key is the caller's
// identifier (a student id); it is never used to break ties.
type Ranked struct {
	Key    string
	GPA    Points
	Total  Marks
	Passed bool
}

// Rank returns the position of each entry by key: passed before failed, then
// GPA descending, then total marks descending. Students equal on all three
// share a position; no arbitrary tie-breaker is applied.
func Rank(entries []Ranked, mode RankMode) map[string]int {
	sorted := append([]Ranked(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return better(sorted[i], sorted[j]) })
	out := make(map[string]int, len(sorted))
	position := 0
	for i, e := range sorted {
		if i == 0 || better(sorted[i-1], e) {
			if mode == Dense {
				position++
			} else {
				position = i + 1
			}
		}
		out[e.Key] = position
	}
	return out
}

func better(a, b Ranked) bool {
	if a.Passed != b.Passed {
		return a.Passed
	}
	if a.GPA != b.GPA {
		return a.GPA > b.GPA
	}
	return a.Total > b.Total
}
