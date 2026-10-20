package provision

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// Keys the provisioner sets in every school's environment file. Every other
// key keeps the template's value.
const (
	keyProject     = "COMPOSE_PROJECT_NAME"
	keyProfiles    = "COMPOSE_PROFILES"
	keyDomain      = "SCHOOL_DOMAIN"
	keyACMEEmail   = "ACME_EMAIL"
	keyDBPassword  = "POSTGRES_PASSWORD"
	keySigningKey  = "JWT_SIGNING_KEY_B64"
	keyWebPort     = "WEB_PORT"
	edgeProfile    = "edge"
	dbPasswordSize = 24 // bytes; 48 hex characters, safe inside DATABASE_URL
	signingKeySize = 32 // bytes; the backend requires at least 32
)

var providedKeys = []string{keyProject, keyProfiles, keyDomain, keyACMEEmail, keyDBPassword, keySigningKey, keyWebPort}

// secrets are generated per school and only ever written to its environment
// file; nothing prints them.
type secrets struct {
	dbPassword string
	signingKey string
}

func newSecrets(random io.Reader) (secrets, error) {
	db := make([]byte, dbPasswordSize)
	key := make([]byte, signingKeySize)
	if _, err := io.ReadFull(random, db); err != nil {
		return secrets{}, fmt.Errorf("generate database password: %w", err)
	}
	if _, err := io.ReadFull(random, key); err != nil {
		return secrets{}, fmt.Errorf("generate signing key: %w", err)
	}
	return secrets{dbPassword: hex.EncodeToString(db), signingKey: base64.StdEncoding.EncodeToString(key)}, nil
}

// defaultRandom is the operating system's cryptographically secure source.
var defaultRandom io.Reader = rand.Reader

type envLine struct {
	key, value string // empty key: a comment or blank line kept as text
	text       string
}

func parseEnv(text string) ([]envLine, error) {
	var lines []envLine
	scanner := bufio.NewScanner(strings.NewReader(text))
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, envLine{text: line})
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("line %d is not KEY=value", n)
		}
		lines = append(lines, envLine{key: key, value: value})
	}
	return lines, scanner.Err()
}

func envValues(lines []envLine) map[string]string {
	values := map[string]string{}
	for _, line := range lines {
		if line.key != "" {
			values[line.key] = line.value
		}
	}
	return values
}

// renderEnv fills the template with a school's values. Every key the
// provisioner sets must exist in the template, and every other key must
// already have a value there, so a new required setting added to the
// template is never silently left empty.
func renderEnv(template []envLine, set map[string]string, header []string) (string, error) {
	present := map[string]bool{}
	var b strings.Builder
	for _, line := range header {
		b.WriteString("# " + line + "\n")
	}
	b.WriteString("\n")
	for _, line := range template {
		if line.key == "" {
			b.WriteString(line.text + "\n")
			continue
		}
		present[line.key] = true
		value, ok := set[line.key]
		if !ok {
			value = line.value
			if strings.TrimSpace(value) == "" {
				return "", fmt.Errorf("the template has no value for %s and the provisioner does not set it; add a default to deploy/school.env.example", line.key)
			}
		}
		if strings.ContainsAny(value, "\r\n") {
			return "", fmt.Errorf("value for %s contains a line break", line.key)
		}
		b.WriteString(line.key + "=" + value + "\n")
	}
	for _, key := range providedKeys {
		if !present[key] {
			return "", fmt.Errorf("deploy/school.env.example has no %s line", key)
		}
	}
	return b.String(), nil
}

// composeSections reads the top-level keys under "services:" and
// "volumes:" of a Compose file, enough to name the Docker resources a
// school's project will own, without a YAML dependency.
func composeSections(path string) (services, volumes []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	section := ""
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if raw == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		if !strings.HasPrefix(raw, " ") {
			section = strings.TrimSuffix(strings.TrimSpace(raw), ":")
			continue
		}
		if strings.HasPrefix(raw, "  ") && !strings.HasPrefix(raw, "   ") && strings.HasSuffix(strings.TrimSpace(raw), ":") {
			name := strings.TrimSuffix(strings.TrimSpace(raw), ":")
			switch section {
			case "services":
				services = append(services, name)
			case "volumes":
				volumes = append(volumes, name)
			}
		}
	}
	if len(services) == 0 || len(volumes) == 0 {
		return nil, nil, fmt.Errorf("%s has no services or volumes", path)
	}
	return services, volumes, nil
}
