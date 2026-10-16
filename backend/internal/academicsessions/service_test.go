package academicsessions

import "testing"

func TestNormalizeSession(t *testing.T) {
	tests := []struct {
		name  string
		input Input
		valid bool
		key   string
	}{
		{"single year", Input{"2026", "2026-01-01", "2026-12-31"}, true, "2026"},
		{"year range", Input{"2026–2027", "2026-07-01", "2027-06-30"}, true, "2026-2027"},
		{"hyphen variant", Input{" 2026-2027 ", "2026-07-01", "2027-06-30"}, true, "2026-2027"},
		{"backward dates", Input{"2026", "2026-12-31", "2026-01-01"}, false, ""},
		{"same date", Input{"2026", "2026-01-01", "2026-01-01"}, false, ""},
		{"invalid date", Input{"2026", "2026-02-30", "2026-12-31"}, false, ""},
		{"invalid range", Input{"2026–2028", "2026-01-01", "2028-12-31"}, false, ""},
		{"arbitrary name", Input{"School A", "2026-01-01", "2026-12-31"}, false, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, key, err := normalize(test.input)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if test.valid && (key != test.key || value.Name == "") {
				t.Fatalf("unexpected normalized value: %#v %q", value, key)
			}
		})
	}
}
