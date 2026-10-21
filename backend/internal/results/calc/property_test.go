package calc

import (
	"math"
	"math/big"
	"math/rand"
	"testing"
)

// atExactEdge reports whether marks/full is exactly a band's lower edge.
func atExactEdge(marks, full Marks, s Scheme) bool {
	for _, b := range s.bands {
		if int64(marks)*100 == int64(b.Min)*int64(full) && b.Min > 0 {
			return true
		}
	}
	return false
}

var commonFull = []int64{100, 50, 25, 75, 20, 30, 40, 60, 70, 80, 10, 15}

// randomSubject draws marks out of full, biased towards exact band edges so
// the boundary behaviour is exercised often.
func randomSubject(r *rand.Rand, optional bool) Subject {
	var full Marks
	switch r.Intn(4) {
	case 0, 1:
		full = Marks(commonFull[r.Intn(len(commonFull))] * 100)
	case 2:
		full = Marks((1 + r.Int63n(200)) * 100) // irregular whole full marks
	default:
		full = Marks(1 + r.Int63n(20000)) // irregular two-decimal full marks
	}
	marks := Marks(r.Int63n(int64(full) + 1))
	if r.Intn(5) == 0 { // an exact edge, when it is a two-decimal mark
		e := int64(edges[r.Intn(len(edges))])
		if e*int64(full)%100 == 0 {
			marks = Marks(e * int64(full) / 100)
		}
	}
	pass := Marks(int64(full) * 33 / 100)
	if r.Intn(4) == 0 {
		pass = Marks(r.Int63n(int64(full) + 1))
	}
	return Subject{Marks: marks, Full: full, Pass: pass, Optional: optional}
}

// TestEquivalenceWithLegacyEngine runs both engines on random results. They
// must agree on every result in which no subject sits exactly on a band edge;
// where they differ, the difference must come from such a subject, with the
// exact engine giving the higher (correct) band.
func TestEquivalenceWithLegacyEngine(t *testing.T) {
	s := bdDefault()
	r := rand.New(rand.NewSource(20261021))
	cases, differing := 200000, 0
	for i := 0; i < cases; i++ {
		n := 1 + r.Intn(9)
		subjects := make([]Subject, 0, n+1)
		for j := 0; j < n; j++ {
			subjects = append(subjects, randomSubject(r, false))
		}
		if r.Intn(2) == 0 {
			subjects = append(subjects, randomSubject(r, true))
		}
		legacy := make([]legacySubject, len(subjects))
		for j, sub := range subjects {
			legacy[j] = legacySubject{sub.Marks.Float(), sub.Full.Float(), sub.Pass.Float(), sub.Optional}
		}
		exact, err := Calculate(subjects, s, Policy{})
		lgpa, lpassed, lgrades, ok := legacyGPA(legacy, bdDefaultLegacy)
		if err != nil || !ok {
			t.Fatalf("case %d: exact %v, legacy ok %v", i, err, ok)
		}
		same := lpassed == exact.Passed && math.Round(lgpa*100) == float64(exact.GPA)
		for j := range subjects {
			same = same && lgrades[j].Name == exact.Grades[j].Grade && lgrades[j].Passed == exact.Grades[j].Passed
		}
		if same {
			continue
		}
		differing++
		explained := false
		for j, sub := range subjects {
			if lgrades[j].Name != exact.Grades[j].Grade {
				if !atExactEdge(sub.Marks, sub.Full, s) || lgrades[j].Point >= exact.Grades[j].Point.Float() {
					t.Fatalf("case %d subject %d: %s/%s legacy %s exact %s, not an exact-edge correction", i, j, sub.Marks, sub.Full, lgrades[j].Name, exact.Grades[j].Grade)
				}
				explained = true
			}
		}
		if !explained {
			t.Fatalf("case %d: results differ (legacy %.2f %v, exact %s %v) with identical subject grades", i, lgpa, lpassed, exact.GPA, exact.Passed)
		}
	}
	t.Logf("%d random results: %d differ, every one by an exact band-edge correction", cases, differing)
}

// TestGPARoundingAgreesWithLegacy enumerates every combination of passing
// grade points for 1–9 required subjects, with and without an optional
// subject, and compares the exact half-up GPA with the float formula.
func TestGPARoundingAgreesWithLegacy(t *testing.T) {
	points := []Points{500, 400, 350, 300, 200, 100}
	optional := []Points{-1, 0, 100, 200, 300, 350, 400, 500} // -1: none
	compared, differing := 0, 0
	var walk func(start, left int, sum Points, n int)
	walk = func(start, left int, sum Points, n int) {
		if left == 0 {
			for _, o := range optional {
				bonus := Points(0)
				if o > 200 {
					bonus = o - 200
				}
				exact := big.NewRat(int64(sum+bonus), int64(n))
				if exact.Cmp(big.NewRat(500, 1)) > 0 {
					exact.SetInt64(500)
				}
				want := roundRat(exact, 1, HalfUp)
				legacy := math.Round(math.Min(5, (float64(sum)/100+float64(bonus)/100)/float64(n))*100) / 100
				compared++
				if int64(math.Round(legacy*100)) != want {
					differing++
					t.Logf("n=%d points=%s bonus=%s: legacy %.2f exact %s", n, sum, bonus, legacy, Points(want))
				}
			}
			return
		}
		for i := start; i < len(points); i++ {
			walk(i, left-1, sum+points[i], n)
		}
	}
	for n := 1; n <= 9; n++ {
		walk(0, n, 0, n)
	}
	t.Logf("%d GPA combinations compared, %d differ", compared, differing)
	if differing != 0 {
		t.Fatalf("the float GPA formula disagrees with exact half-up rounding in %d cases", differing)
	}
}

func TestGradePointIsMonotonicInMarks(t *testing.T) {
	s := bdDefault()
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 100000; i++ {
		full := Marks(1 + r.Int63n(100000))
		a := Marks(r.Int63n(int64(full) + 1))
		b := a + Marks(r.Int63n(int64(full-a)+1))
		ga, _ := s.Band(a, full)
		gb, _ := s.Band(b, full)
		if gb.Point < ga.Point {
			t.Fatalf("%s/%s is %s but %s/%s is %s", a, full, ga.Grade, b, full, gb.Grade)
		}
	}
}

func TestOrderDoesNotMatter(t *testing.T) {
	s := bdDefault()
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 20000; i++ {
		subjects := []Subject{randomSubject(r, false), randomSubject(r, false), randomSubject(r, false), randomSubject(r, true)}
		a, _ := Calculate(subjects, s, Policy{})
		r.Shuffle(len(subjects), func(x, y int) { subjects[x], subjects[y] = subjects[y], subjects[x] })
		b, _ := Calculate(subjects, s, Policy{})
		if a.GPA != b.GPA || a.Passed != b.Passed || a.Total != b.Total || a.Full != b.Full {
			t.Fatalf("subject order changed the result: %+v vs %+v", a, b)
		}
		sources := []Score{{Marks(r.Int63n(2001)), 2000, Present}, {Marks(r.Int63n(2501)), 2500, Present}, {Marks(r.Int63n(1001)), 1000, Present}}
		for _, combine := range []Combine{Sum, Average, BestN} {
			c := Contribution{Sources: sources, Combine: combine, N: 2, Worth: 2000}
			x, _ := Contribute(c, AbsentScoresZero)
			c.Sources = []Score{sources[2], sources[0], sources[1]}
			y, _ := Contribute(c, AbsentScoresZero)
			if x.Value.Cmp(y.Value) != 0 {
				t.Fatalf("source order changed combine %d: %s vs %s", combine, x.Value, y.Value)
			}
		}
		avg, _ := Contribute(Contribution{Sources: sources, Combine: Average, Worth: 2000}, AbsentScoresZero)
		best, _ := Contribute(Contribution{Sources: sources, Combine: BestN, N: 2, Worth: 2000}, AbsentScoresZero)
		if best.Value.Cmp(avg.Value) < 0 {
			t.Fatalf("best two below the average: %s < %s", best.Value, avg.Value)
		}
	}
}

func TestRankIgnoresInputOrder(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 2000; i++ {
		entries := make([]Ranked, 30)
		for j := range entries {
			passed := r.Intn(5) != 0
			gpa := Points(0)
			if passed {
				gpa = Points(100 + r.Int63n(5)*100)
			}
			entries[j] = Ranked{Key: string(rune('a' + j)), GPA: gpa, Total: Marks(r.Int63n(4) * 1000), Passed: passed}
		}
		for _, mode := range []RankMode{Competition, Dense} {
			a := Rank(entries, mode)
			r.Shuffle(len(entries), func(x, y int) { entries[x], entries[y] = entries[y], entries[x] })
			b := Rank(entries, mode)
			for k, v := range a {
				if b[k] != v {
					t.Fatalf("mode %d: %s at %d then %d", mode, k, v, b[k])
				}
			}
		}
	}
}
