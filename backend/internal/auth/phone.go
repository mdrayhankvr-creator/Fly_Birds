package auth

import (
	"regexp"
	"strings"
)

// Bangladeshi mobile numbers as a sign-in identifier. They are written in
// several ways — 01711000000, 8801711000000, +880 1711-000000 — and are all
// stored and compared in one canonical form, +8801XXXXXXXXX.

// localPhone is the number without its country code: 1, an operator digit
// (3–9, so 013–019 with the leading zero) and eight more digits.
var localPhone = regexp.MustCompile(`^1[3-9][0-9]{8}$`)

var phoneSeparators = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "", " ", "")

var phoneBanglaDigits = strings.NewReplacer("০", "0", "১", "1", "২", "2", "৩", "3", "৪", "4", "৫", "5", "৬", "6", "৭", "7", "৮", "8", "৯", "9")

// NormalizePhone turns a written Bangladeshi mobile number into the stored
// form +8801XXXXXXXXX. It accepts 01XXXXXXXXX, 8801XXXXXXXXX and
// +8801XXXXXXXXX, with spaces, dashes or brackets between the digits, and
// Bangla digits. Anything else — including a bare ten-digit number, which a
// Student ID could also be — is not a phone number.
func NormalizePhone(raw string) (string, bool) {
	s := phoneBanglaDigits.Replace(phoneSeparators.Replace(strings.TrimSpace(raw)))
	switch {
	case strings.HasPrefix(s, "+880"):
		s = s[len("+880"):]
	case strings.HasPrefix(s, "880"):
		s = s[len("880"):]
	case strings.HasPrefix(s, "0"):
		s = s[len("0"):]
	default:
		return "", false
	}
	if !localPhone.MatchString(s) {
		return "", false
	}
	return "+880" + s, true
}

// ValidPhone reports whether raw is a Bangladeshi mobile number.
func ValidPhone(raw string) bool {
	_, ok := NormalizePhone(raw)
	return ok
}

// FormatPhone writes a stored number for people to read: 01711-000000.
func FormatPhone(stored string) string {
	if !strings.HasPrefix(stored, "+880") || len(stored) != len("+8801711000000") {
		return stored
	}
	local := "0" + stored[len("+880"):]
	return local[:5] + "-" + local[5:]
}
