package school

import (
	"errors"
	"testing"
)

func TestValidateSettings(t *testing.T) {
	base := SettingsInput{NameEN: "Example School", NameBN: "উদাহরণ বিদ্যালয়", PreferredLanguage: "bn", PrimaryColor: "#0F766E", SecondaryColor: "#334155", DashboardTheme: "light", GradingSchemeID: "bd_default"}
	if _, err := ValidateSettings(base); err != nil {
		t.Fatal(err)
	}
	cases := []SettingsInput{
		func() SettingsInput { x := base; x.NameEN = ""; return x }(),
		func() SettingsInput { x := base; x.PrimaryColor = "#FFFFFF"; return x }(),
		func() SettingsInput { x := base; x.LogoURL = "javascript:alert(1)"; return x }(),
		func() SettingsInput { x := base; x.LogoURL = "https://localhost/logo.png"; return x }(),
		func() SettingsInput { x := base; x.LogoURL = "https://school.example/logo.svg"; return x }(),
		func() SettingsInput { x := base; x.LogoURL = "https://school.example/logo.png?"; return x }(),
		func() SettingsInput { x := base; x.PreferredLanguage = "xx"; return x }(),
		func() SettingsInput { x := base; x.Email = "invalid"; return x }(),
	}
	for _, input := range cases {
		if _, err := ValidateSettings(input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid settings accepted: %+v", input)
		}
	}
	base.LogoURL = "https://assets.example.org/school.png"
	if _, err := ValidateSettings(base); err != nil {
		t.Fatalf("safe logo URL rejected: %v", err)
	}
}

func defaultBands() []GradeBand {
	return []GradeBand{
		{Grade: "A+", MinMarks: 80, MaxMarks: 100, GPA: 5},
		{Grade: "A", MinMarks: 70, MaxMarks: 79, GPA: 4},
		{Grade: "A-", MinMarks: 60, MaxMarks: 69, GPA: 3.5},
		{Grade: "B", MinMarks: 50, MaxMarks: 59, GPA: 3},
		{Grade: "C", MinMarks: 40, MaxMarks: 49, GPA: 2},
		{Grade: "D", MinMarks: 33, MaxMarks: 39, GPA: 1},
		{Grade: "F", MinMarks: 0, MaxMarks: 32, GPA: 0},
	}
}
func TestValidateBandsCoverageAndBoundaries(t *testing.T) {
	bands, err := ValidateBands(defaultBands())
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{0: "F", 32: "F", 33: "D", 39: "D", 40: "C", 49: "C", 50: "B", 59: "B", 60: "A-", 69: "A-", 70: "A", 79: "A", 80: "A+", 100: "A+"}
	for mark, grade := range want {
		found := ""
		for _, b := range bands {
			if mark >= b.MinMarks && mark <= b.MaxMarks {
				found = b.Grade
				break
			}
		}
		if found != grade {
			t.Fatalf("mark %d: got %q, want %q", mark, found, grade)
		}
	}
	gap := defaultBands()
	gap[6].MaxMarks = 31
	if _, err := ValidateBands(gap); !errors.Is(err, ErrInvalid) {
		t.Fatal("gap accepted")
	}
	overlap := defaultBands()
	overlap[6].MaxMarks = 33
	if _, err := ValidateBands(overlap); !errors.Is(err, ErrInvalid) {
		t.Fatal("overlap accepted")
	}
	badGPA := defaultBands()
	badGPA[0].GPA = 5.01
	if _, err := ValidateBands(badGPA); !errors.Is(err, ErrInvalid) {
		t.Fatal("GPA greater than five accepted")
	}
	tooPrecise := defaultBands()
	tooPrecise[0].GPA = 4.999
	if _, err := ValidateBands(tooPrecise); !errors.Is(err, ErrInvalid) {
		t.Fatal("GPA with three decimals accepted")
	}
}
