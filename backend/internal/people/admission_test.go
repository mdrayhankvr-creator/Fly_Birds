package people

import (
	"regexp"
	"testing"
)

func TestPhoneDigits(t *testing.T) {
	for in, want := range map[string]string{
		"01711-000000":     "01711000000",
		"+880 1711 000000": "01711000000",
		"8801711000000":    "01711000000",
		"০১৭১১০০০০০০":      "01711000000",
		"(02) 955-1234":    "029551234",
		"":                 "",
	} {
		if got := PhoneDigits(in); got != want {
			t.Errorf("PhoneDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextAdmissionNumber(t *testing.T) {
	none := func(string) bool { return false }
	cases := []struct {
		name             string
		recent, existing []string
		taken            func(string) bool
		want             string
	}{
		{"continues the padded number", []string{"ADM-2026-0041", "ADM-2026-0040"}, nil, none, "ADM-2026-0042"},
		{"above the highest with the prefix", []string{"ADM-2026-0041", "ADM-2026-0040"}, []string{"ADM-2026-0107"}, none, "ADM-2026-0108"},
		{"skips a number in use", []string{"S12", "S11"}, nil, func(v string) bool { return v == "S13" }, "S14"},
		{"no pattern from one admission", []string{"ADM-1"}, nil, none, ""},
		{"no pattern when prefixes differ", []string{"A-1", "B-2"}, nil, none, ""},
		{"no pattern without digits", []string{"ALPHA", "BETA"}, nil, none, ""},
		{"wider numbers grow", []string{"R-99", "R-98"}, nil, none, "R-100"},
	}
	for _, c := range cases {
		if got := NextAdmissionNumber(c.recent, c.existing, c.taken); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestInitialPasswordShape(t *testing.T) {
	shape := regexp.MustCompile(`^[a-zA-Z2-9]{4}-[a-zA-Z2-9]{4}-[a-zA-Z2-9]{4}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := newInitialPassword()
		if err != nil {
			t.Fatal(err)
		}
		if !shape.MatchString(p) || len(p) < 12 {
			t.Fatalf("password %q has the wrong shape", p)
		}
		for _, r := range p {
			if r == '0' || r == 'O' || r == 'o' || r == '1' || r == 'l' || r == 'I' || r == '5' || r == 'S' {
				t.Fatalf("password %q has a look-alike character", p)
			}
		}
		if seen[p] {
			t.Fatalf("password %q repeated", p)
		}
		seen[p] = true
	}
}
