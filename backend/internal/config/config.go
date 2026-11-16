package config

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	// PublicBaseURL is the school's public address (PUBLIC_BASE_URL), used in
	// the verification QR codes printed on marksheets; empty prints the
	// document code only.
	PublicBaseURL string
	// TypstBin and DocumentFontsDir locate the document renderer
	// (TYPST_BIN, DOCUMENT_FONTS_DIR); without them documents are off.
	TypstBin         string
	DocumentFontsDir string
	// MigrationTimeout bounds applying the schema at startup
	// (MIGRATION_TIMEOUT, default 5m).
	MigrationTimeout time.Duration
	// SMTP* describe the mail relay that sends the "forgot password" link
	// (SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM,
	// SMTP_FROM_NAME, SMTP_SECURITY). An empty SMTP_HOST leaves email off:
	// the sign-in page then hides "Forgot password" and keeps telling people
	// to ask the school office, and an administrator resets passwords as
	// before. A configured relay also needs PUBLIC_BASE_URL, because that is
	// the address the emailed link points at.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	// SMTPSecurity is starttls (default, port 587), tls (port 465) or none.
	// none is for a fake relay in a test and refuses a password.
	SMTPSecurity string
}

// EmailEnabled reports whether a mail relay is configured.
func (c Config) EmailEnabled() bool { return c.SMTPHost != "" }

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
	publicBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	if publicBaseURL != "" {
		u, err := url.Parse(publicBaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, fmt.Errorf("PUBLIC_BASE_URL must be an http(s) address such as https://school.example.edu.bd")
		}
	}
	migrationTimeout := 5 * time.Minute
	if raw := os.Getenv("MIGRATION_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 10*time.Second {
			return Config{}, fmt.Errorf("MIGRATION_TIMEOUT must be a duration of at least 10s, such as 5m")
		}
		migrationTimeout = parsed
	}
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	smtpPort := 587
	smtpSecurity := strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_SECURITY")))
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	if smtpHost != "" {
		if raw := os.Getenv("SMTP_PORT"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 65535 {
				return Config{}, fmt.Errorf("SMTP_PORT must be a number between 1 and 65535, such as 587")
			}
			smtpPort = parsed
		}
		if smtpSecurity == "" {
			smtpSecurity = "starttls"
		}
		if smtpSecurity != "starttls" && smtpSecurity != "tls" && smtpSecurity != "none" {
			return Config{}, fmt.Errorf("SMTP_SECURITY must be starttls, tls or none")
		}
		if smtpSecurity == "none" && smtpPassword != "" {
			return Config{}, fmt.Errorf("SMTP_SECURITY=none sends in the clear, so SMTP_PASSWORD may not be set; it is only for a fake relay in a test")
		}
		if (smtpUser == "") != (smtpPassword == "") {
			return Config{}, fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must be set together")
		}
		parsed, err := mail.ParseAddress(smtpFrom)
		if err != nil || parsed.Address != smtpFrom {
			return Config{}, fmt.Errorf("SMTP_FROM must be the address the school sends from, such as noreply@example.edu.bd")
		}
		if publicBaseURL == "" {
			return Config{}, fmt.Errorf("PUBLIC_BASE_URL is required when SMTP_HOST is set: it is the address the emailed reset link points at")
		}
	} else if os.Getenv("SMTP_PORT") != "" || smtpFrom != "" || smtpUser != "" || smtpPassword != "" || smtpSecurity != "" {
		return Config{}, fmt.Errorf("SMTP_HOST is required to send email; remove the other SMTP_ settings or set it")
	}
	fontsDir := os.Getenv("DOCUMENT_FONTS_DIR")
	if fontsDir == "" {
		fontsDir = "/app/fonts"
	}
	return Config{
		SMTPHost:         smtpHost,
		SMTPPort:         smtpPort,
		SMTPUsername:     smtpUser,
		SMTPPassword:     smtpPassword,
		SMTPFrom:         smtpFrom,
		SMTPFromName:     strings.TrimSpace(os.Getenv("SMTP_FROM_NAME")),
		SMTPSecurity:     smtpSecurity,
		PublicBaseURL:    publicBaseURL,
		MigrationTimeout: migrationTimeout,
		TypstBin:         os.Getenv("TYPST_BIN"),
		DocumentFontsDir: fontsDir,
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
