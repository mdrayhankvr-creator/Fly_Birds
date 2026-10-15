package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtIssuer   = "fly-birds"
	jwtAudience = "fly-birds-api"
)

type TokenManager struct{ key []byte }

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewTokenManager(key []byte) (*TokenManager, error) {
	if len(key) < 32 {
		return nil, errors.New("JWT signing key must be at least 32 bytes")
	}
	return &TokenManager{key: append([]byte(nil), key...)}, nil
}

func (m *TokenManager) Issue(userID, sessionID string, now time.Time) (string, error) {
	jti, err := randomUUID()
	if err != nil {
		return "", err
	}
	claims := accessClaims{SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Audience:  jwt.ClaimStrings{jwtAudience},
		Subject:   userID,
		ID:        jti,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessLifetime)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.key)
}

func (m *TokenManager) Verify(raw string) (string, string, error) {
	claims := new(accessClaims)
	token, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) { return m.key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithStrictDecoding(),
	)
	if err != nil || !token.Valid || claims.Subject == "" || claims.ID == "" || claims.SessionID == "" {
		return "", "", ErrInvalidToken
	}
	return claims.Subject, claims.SessionID, nil
}

func randomOpaque() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashOpaque(value string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return nil, ErrInvalidSession
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

func randomUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}
