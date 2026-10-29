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
	// FileStorageDir holds uploaded attachments (FILE_STORAGE_DIR).
	FileStorageDir string
	// NotifyInterval is how often notifications are dispatched; 0 disables it.
	NotifyInterval time.Duration
	// BackupDir holds backup archives (BACKUP_DIR); keep it off the web root.
	BackupDir string
	// BackupRetention is how many completed archives are kept (BACKUP_RETENTION).
	BackupRetention int
	// BackupInterval schedules automatic backups (BACKUP_INTERVAL); 0 disables.
	BackupInterval time.Duration
	// PaymentSimulationAllowed lets an administrator switch on simulated
	// bKash/Nagad payments (PAYMENT_SIMULATION_ALLOWED). Only for disposable
	// test installations; off by default.
	PaymentSimulationAllowed bool
	// DemoQuickLogin shows one-click sign-in for the seed-demo accounts
	// (DEMO_QUICK_LOGIN). Only for the owner's local testing; off by default
	// and never set by the production Compose file or school provisioning.
	DemoQuickLogin bool
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
	storageDir := os.Getenv("FILE_STORAGE_DIR")
	if storageDir == "" {
		storageDir = "data/uploads"
	}
	notifyInterval := time.Minute
	if raw := os.Getenv("NOTIFY_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 0 || (parsed > 0 && parsed < 10*time.Second) {
			return Config{}, fmt.Errorf("NOTIFY_INTERVAL must be 0 or a duration of at least 10s")
		}
		notifyInterval = parsed
	}
	backupDir := os.Getenv("BACKUP_DIR")
	if backupDir == "" {
		backupDir = "data/backups"
	}
	retention := 10
	if raw := os.Getenv("BACKUP_RETENTION"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 365 {
			return Config{}, fmt.Errorf("BACKUP_RETENTION must be between 1 and 365")
		}
		retention = parsed
	}
	var backupInterval time.Duration
	if raw := os.Getenv("BACKUP_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 0 || (parsed > 0 && parsed < time.Hour) {
			return Config{}, fmt.Errorf("BACKUP_INTERVAL must be 0 or a duration of at least 1h")
		}
		backupInterval = parsed
	}
	simulation := false
	if raw := os.Getenv("PAYMENT_SIMULATION_ALLOWED"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("PAYMENT_SIMULATION_ALLOWED must be true or false")
		}
		simulation = parsed
	}
	demoQuickLogin := false
	if raw := os.Getenv("DEMO_QUICK_LOGIN"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("DEMO_QUICK_LOGIN must be true or false")
		}
		demoQuickLogin = parsed
	}
	return Config{
		Port:             port,
		DatabaseURL:      databaseURL,
		JWTSigningKey:    signingKey,
		AuthCookieSecure: cookieSecure,
		ShutdownTimeout:  10 * time.Second,
		FileStorageDir:   storageDir,
		NotifyInterval:   notifyInterval,
		BackupDir:        backupDir,
		BackupRetention:  retention,
		BackupInterval:   backupInterval,

		PaymentSimulationAllowed: simulation,
		DemoQuickLogin:           demoQuickLogin,
	}, nil
}
