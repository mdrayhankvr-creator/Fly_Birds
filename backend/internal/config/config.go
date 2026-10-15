package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             int
	DatabaseURL      string
	JWTSigningKey    []byte
	AuthCookieSecure bool
	ShutdownTimeout  time.Duration
}

func Load() (Config, error) {
	port := 8080
	if raw := os.Getenv("PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number between 1 and 65535")
		}
		port = parsed
	}

	databaseURL := os.Getenv("DATABASE_URL")
	var signingKey []byte
	if raw := os.Getenv("JWT_SIGNING_KEY_B64"); raw != "" {
		var err error
		signingKey, err = base64.StdEncoding.DecodeString(raw)
		if err != nil || len(signingKey) < 32 {
			return Config{}, fmt.Errorf("JWT_SIGNING_KEY_B64 must be base64 encoding of at least 32 random bytes")
		}
	} else if databaseURL != "" {
		return Config{}, fmt.Errorf("JWT_SIGNING_KEY_B64 is required when DATABASE_URL is configured")
	}
	cookieSecure := true
	if raw := os.Getenv("AUTH_COOKIE_SECURE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE must be true or false")
		}
		cookieSecure = parsed
	}
	return Config{
		Port:             port,
		DatabaseURL:      databaseURL,
		JWTSigningKey:    signingKey,
		AuthCookieSecure: cookieSecure,
		ShutdownTimeout:  10 * time.Second,
	}, nil
}
