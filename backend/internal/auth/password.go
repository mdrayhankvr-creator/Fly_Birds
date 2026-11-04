package auth

import (
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

// temporaryCost hashes the temporary passwords the system generates for new
// student logins (TemporaryPassword: 12 random characters from 50, about 67
// bits). Their randomness, not the hash cost, is what protects them, and the
// account must replace them at the first sign-in, when the new password is
// hashed at passwordCost. The lower cost lets a section or a spreadsheet of
// students get logins in seconds. Login pads its work so the cost of an
// account's hash does not show in the response time.
const temporaryCost = 8

// HashTemporaryPassword hashes a password made by TemporaryPassword.
func HashTemporaryPassword(password string) (string, error) {
	if !utf8.ValidString(password) || len(password) < 12 || len(password) > 72 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), temporaryCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func HashPassword(password string) (string, error) {
	if !utf8.ValidString(password) || len(password) < 12 || len(password) > 72 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
