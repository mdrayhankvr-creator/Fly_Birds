package auth

import (
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

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
