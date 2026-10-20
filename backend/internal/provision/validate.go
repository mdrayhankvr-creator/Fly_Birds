// Package provision prepares the configuration of one school's isolated
// deployment: a Docker Compose project with its own database, storage,
// backups, secrets and domain, described by a Git-ignored environment file
// rendered from deploy/school.env.example. It never starts containers.
package provision

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const (
	minCodeLength = 3
	maxCodeLength = 32
	minWebPort    = 1024
	maxWebPort    = 65535
	// FirstWebPort is where automatic WEB_PORT assignment starts; it matches
	// the template's default.
	FirstWebPort = 8081
)

// A school code names the Compose project, so it prefixes every container,
// volume and network of the school. It has no underscore, which keeps the
// "<code>_" volume prefixes of two schools from ever overlapping, and no
// characters that need quoting in a shell or a file name.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateCode checks a school code: 3–32 characters of lower-case letters,
// digits and single hyphens, starting with a letter and not ending with a
// hyphen.
func ValidateCode(code string) error {
	if len(code) < minCodeLength || len(code) > maxCodeLength {
		return fmt.Errorf("school code %q must be %d to %d characters long", code, minCodeLength, maxCodeLength)
	}
	if !codePattern.MatchString(code) {
		return fmt.Errorf("school code %q may contain only lower-case letters, digits and single hyphens, must start with a letter and must not end with a hyphen", code)
	}
	return nil
}

var (
	labelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	tldPattern   = regexp.MustCompile(`^([a-z]{2,63}|xn--[a-z0-9-]{1,59})$`)
)

// NormalizeDomain lower-cases and checks a school's host name, for example
// "greenvalley.flybirds.com" under any parent domain. Only plain DNS host
// names are accepted: no scheme, port, path, wildcard, IP address, trailing
// dot, spaces or other characters that could change the Caddyfile or the
// environment file. "localhost" and names under ".localhost" are accepted for
// local trials.
func NormalizeDomain(raw string) (string, error) {
	domain := strings.ToLower(raw)
	if domain == "" {
		return "", fmt.Errorf("a school domain is required, for example school.example.com")
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("school domain is longer than 253 characters")
	}
	if domain == "localhost" {
		return domain, nil
	}
	if net.ParseIP(domain) != nil {
		return "", fmt.Errorf("school domain %q is an IP address; use a DNS name", raw)
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("school domain %q must be a full host name such as school.example.com", raw)
	}
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return "", fmt.Errorf("school domain %q is not a valid host name: each part must be 1–63 letters, digits or inner hyphens", raw)
		}
	}
	if !tldPattern.MatchString(labels[len(labels)-1]) {
		return "", fmt.Errorf("school domain %q must end in a top-level domain such as .com", raw)
	}
	return domain, nil
}

// IsLocalDomain reports whether a domain only resolves on this machine.
// Caddy serves such names with its own local certificate authority, so they
// need no public DNS and no certificate contact address.
func IsLocalDomain(domain string) bool {
	return domain == "localhost" || strings.HasSuffix(domain, ".localhost")
}

var emailPattern = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,64}@([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// ValidateACMEEmail checks the certificate contact address. Let's Encrypt
// refuses example domains, so the template's placeholder is rejected here
// rather than when Caddy first requests a certificate.
func ValidateACMEEmail(email string) error {
	if len(email) > 254 || !emailPattern.MatchString(email) {
		return fmt.Errorf("certificate contact %q is not a valid email address", email)
	}
	domain := email[strings.LastIndex(email, "@")+1:]
	for _, label := range strings.Split(domain, ".") {
		if label == "example" || label == "invalid" || label == "test" || label == "localhost" {
			return fmt.Errorf("certificate contact %q uses a reserved domain; give a mailbox that receives certificate notices", email)
		}
	}
	return nil
}

// ValidateWebPort checks a WEB_PORT value: an unprivileged TCP port.
func ValidateWebPort(port int) error {
	if port < minWebPort || port > maxWebPort {
		return fmt.Errorf("web port %d must be between %d and %d", port, minWebPort, maxWebPort)
	}
	return nil
}
