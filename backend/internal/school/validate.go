package school

import (
	"errors"
	"math"
	"net"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid        = errors.New("invalid school configuration")
	ErrNotFound       = errors.New("school resource not found")
	ErrForbidden      = errors.New("school update forbidden")
	ErrCoreModule     = errors.New("core module cannot be disabled")
	ErrModuleDisabled = errors.New("module disabled")
)
var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
var moduleKey = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
var schemeKey = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
var hostLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
var phoneChars = regexp.MustCompile(`^\+?[0-9][0-9 -]{5,18}[0-9]$`)

func colorLuminance(value string) float64 {
	var parts [3]float64
	for i := 0; i < 3; i++ {
		var n int
		for _, c := range value[1+i*2 : 3+i*2] {
			n *= 16
			if c >= '0' && c <= '9' {
				n += int(c - '0')
			} else if c >= 'A' && c <= 'F' {
				n += int(c - 'A' + 10)
			} else {
				n += int(c - 'a' + 10)
			}
		}
		v := float64(n) / 255
		if v <= 0.04045 {
			parts[i] = v / 12.92
		} else {
			parts[i] = math.Pow((v+0.055)/1.055, 2.4)
		}
	}
	return parts[0]*0.2126 + parts[1]*0.7152 + parts[2]*0.0722
}
func validBrandColor(value string) bool {
	return hexColor.MatchString(value) && 1.05/(colorLuminance(value)+0.05) >= 4.5
}

func validLogoURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	host := u.Hostname()
	if host == "" || len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil || strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

func ValidateSettings(input SettingsInput) (SettingsInput, error) {
	input.NameEN = strings.TrimSpace(input.NameEN)
	input.NameBN = strings.TrimSpace(input.NameBN)
	input.Address = strings.TrimSpace(input.Address)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.PrimaryColor = strings.ToUpper(input.PrimaryColor)
	input.SecondaryColor = strings.ToUpper(input.SecondaryColor)
	if input.NameEN == "" || utf8.RuneCountInString(input.NameEN) > 120 || input.NameBN == "" || utf8.RuneCountInString(input.NameBN) > 120 || utf8.RuneCountInString(input.Address) > 500 {
		return SettingsInput{}, ErrInvalid
	}
	for _, s := range []string{input.NameEN, input.NameBN, input.Address} {
		for _, r := range s {
			if unicode.IsControl(r) && r != '\n' {
				return SettingsInput{}, ErrInvalid
			}
		}
	}
	if input.Phone != "" && (!phoneChars.MatchString(input.Phone) || len(input.Phone) > 20) {
		return SettingsInput{}, ErrInvalid
	}
	if input.Email != "" {
		parsed, err := mail.ParseAddress(input.Email)
		if err != nil || parsed.Address != input.Email || len(input.Email) > 254 {
			return SettingsInput{}, ErrInvalid
		}
	}
	if !validLogoURL(input.LogoURL) || !validBrandColor(input.PrimaryColor) || !validBrandColor(input.SecondaryColor) {
		return SettingsInput{}, ErrInvalid
	}
	if input.PreferredLanguage != "en" && input.PreferredLanguage != "bn" {
		return SettingsInput{}, ErrInvalid
	}
	if input.DashboardTheme != "light" && input.DashboardTheme != "dark" {
		return SettingsInput{}, ErrInvalid
	}
	if !schemeKey.MatchString(input.GradingSchemeID) {
		return SettingsInput{}, ErrInvalid
	}
	for _, tagline := range []*string{input.TaglineEN, input.TaglineBN} {
		if tagline == nil {
			continue
		}
		*tagline = strings.TrimSpace(*tagline)
		if !validLine(*tagline, 160) {
			return SettingsInput{}, ErrInvalid
		}
	}
	return input, nil
}

// validLine accepts one line of text up to max characters, with no control
// characters.
func validLine(value string, max int) bool {
	if utf8.RuneCountInString(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ValidateBands(input []GradeBand) ([]GradeBand, error) {
	if len(input) < 1 || len(input) > 30 {
		return nil, ErrInvalid
	}
	bands := append([]GradeBand(nil), input...)
	seen := map[string]bool{}
	for _, b := range bands {
		label := strings.TrimSpace(b.Grade)
		if label == "" || label != b.Grade || utf8.RuneCountInString(label) > 16 || seen[label] || b.MinMarks < 0 || b.MaxMarks > 100 || b.MinMarks > b.MaxMarks || math.IsNaN(b.GPA) || math.IsInf(b.GPA, 0) || b.GPA < 0 || b.GPA > 5 || math.Abs(math.Round(b.GPA*100)-b.GPA*100) > 1e-6 {
			return nil, ErrInvalid
		}
		for _, r := range label {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '-' {
				return nil, ErrInvalid
			}
		}
		seen[label] = true
	}
	sort.Slice(bands, func(i, j int) bool { return bands[i].MinMarks < bands[j].MinMarks })
	next := 0
	for _, b := range bands {
		if b.MinMarks != next {
			return nil, ErrInvalid
		}
		next = b.MaxMarks + 1
	}
	if next != 101 {
		return nil, ErrInvalid
	}
	return bands, nil
}
func ValidModuleKey(key string) bool { return moduleKey.MatchString(key) }
