package auth

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// PlaceholderDomain is where logins made without an email get their address.
// .invalid is reserved (RFC 2606), so mail never reaches it, and Login
// refuses such addresses: these accounts sign in with their sign-in ID only.
const PlaceholderDomain = "no-email.invalid"

var loginIDPattern = regexp.MustCompile(`^[^\s@]{1,64}$`)

// ValidLoginID reports whether s can be a student sign-in ID: 1 to 64
// characters, no spaces and no "@" (which marks an email address).
func ValidLoginID(s string) bool { return loginIDPattern.MatchString(s) }

// ValidIdentifier reports whether s is an email address, a Bangladeshi
// mobile number or a sign-in ID.
func ValidIdentifier(s string) bool {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "@") {
		_, err := NormalizeEmail(s)
		return err == nil
	}
	if ValidPhone(s) {
		return true
	}
	return ValidLoginID(s)
}

// PlaceholderEmail returns a fresh, unique address under PlaceholderDomain.
func PlaceholderEmail() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "student-" + hex.EncodeToString(b[:]) + "@" + PlaceholderDomain, nil
}

// IsPlaceholderEmail reports whether email is a placeholder address.
func IsPlaceholderEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(email), "@"+PlaceholderDomain)
}
