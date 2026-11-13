package auth

import "testing"

func TestNormalizePhoneAcceptsTheWrittenForms(t *testing.T) {
	const want = "+8801711000000"
	for _, in := range []string{
		"01711000000", "8801711000000", "+8801711000000",
		"+880 1711-000000", "01711-000000", " 01711 000 000 ", "(+880) 1711.000000",
		"০১৭১১০০০০০০", // Bangla digits, as the guide writes them
	} {
		got, ok := NormalizePhone(in)
		if !ok || got != want {
			t.Fatalf("NormalizePhone(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	// Every operator prefix 013–019 is a number; 010, 011 and 012 are not.
	for d := '0'; d <= '9'; d++ {
		in := "01" + string(d) + "11000000"
		_, ok := NormalizePhone(in)
		if want := d >= '3'; ok != want {
			t.Fatalf("NormalizePhone(%q) = %v, want %v", in, ok, want)
		}
	}
}

func TestNormalizePhoneRejectsEverythingElse(t *testing.T) {
	for _, in := range []string{
		"", "   ", "0171100000", "017110000000", // too short, too long
		"1711000000",                                        // no leading zero or country code
		"+9101711000000",                                    // another country
		"DEMO-001", "2609120417", "student-01@example.test", // Student IDs and emails
		"0171100000a", "++8801711000000",
	} {
		if got, ok := NormalizePhone(in); ok {
			t.Fatalf("NormalizePhone(%q) = %q, true; want false", in, got)
		}
	}
}

// The sign-in field takes one value and works out what it is.
func TestIdentifierDetection(t *testing.T) {
	cases := []struct{ in, kind string }{
		{"teacher@example.test", "email"},
		{"01711000000", "phone"},
		{"+880 1711-000000", "phone"},
		{"8801711000000", "phone"},
		{"DEMO-001", "login_id"},
		{"2609120417", "login_id"}, // ten digits: a Student ID, not a number
		{"demo.student@demo.flybirds.test", "email"},
	}
	for _, c := range cases {
		var kind string
		switch {
		case containsAt(c.in):
			kind = "email"
		case ValidPhone(c.in):
			kind = "phone"
		case ValidLoginID(c.in):
			kind = "login_id"
		}
		if kind != c.kind {
			t.Fatalf("%q read as %s, want %s", c.in, kind, c.kind)
		}
		if !ValidIdentifier(c.in) {
			t.Fatalf("%q is not accepted as an identifier", c.in)
		}
	}
}

func containsAt(s string) bool {
	for _, r := range s {
		if r == '@' {
			return true
		}
	}
	return false
}

func TestFormatPhoneForReading(t *testing.T) {
	if got := FormatPhone("+8801711000000"); got != "01711-000000" {
		t.Fatalf("FormatPhone = %q", got)
	}
	if got := FormatPhone(""); got != "" {
		t.Fatalf("FormatPhone(\"\") = %q", got)
	}
}
