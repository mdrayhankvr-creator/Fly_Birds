package documents

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// Verification: each published version printed on a marksheet gets a random
// token; its QR code opens a public page that says whether the result is
// genuine and current. Like the ID card's check, it shows no personal data:
// no name, marks or grades.

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// IsToken reports whether s has the shape of a verification token.
func IsToken(s string) bool { return tokenPattern.MatchString(s) }

// newToken is 128 random bits, base64url without padding (22 characters).
const newToken = `translate(rtrim(encode(decode(replace(gen_random_uuid()::text,'-',''),'hex'),'base64'),'='),'+/','-_')`

// token returns the version's verification token, creating it once.
func (s *Service) token(ctx context.Context, versionID string) (string, error) {
	db := s.db.WithContext(ctx)
	if err := db.Exec("INSERT INTO result_verifications (version_id, token) VALUES (?, "+newToken+") ON CONFLICT (version_id) DO NOTHING", versionID).Error; err != nil {
		return "", err
	}
	var token string
	err := db.Raw("SELECT token FROM result_verifications WHERE version_id = ?", versionID).Row().Scan(&token)
	return token, err
}

// documentCode is a short code printed on the marksheet and shown by the
// check, so a reader can match the paper to the page.
func documentCode(token string) string {
	sum := sha256.Sum256([]byte("flybirds-result:" + token))
	h := strings.ToUpper(hex.EncodeToString(sum[:4]))
	return h[:4] + "-" + h[4:]
}

func qrPNG(url string) ([]byte, error) { return qrcode.Encode(url, qrcode.Medium, 320) }

// Verification is what the public check shows.
type Verification struct {
	// State: current (the latest published version), superseded (corrected
	// since) or withdrawn.
	State        string `json:"state"`
	Code         string `json:"code"`
	Scope        string `json:"scope"`
	Title        string `json:"title"`
	SessionName  string `json:"session_name"`
	ClassName    string `json:"class_name"`
	Version      int    `json:"version"`
	PublishedAt  string `json:"published_at"`
	SchoolNameEN string `json:"school_name_en"`
	SchoolNameBN string `json:"school_name_bn"`
	LogoURL      string `json:"logo_url"`
}

// Verify checks a token from a marksheet's QR code.
func (s *Service) Verify(ctx context.Context, token string) (Verification, error) {
	if !IsToken(token) {
		return Verification{}, ErrNotFound
	}
	var v Verification
	var status string
	err := s.db.WithContext(ctx).Raw(`SELECT v.status, v.scope, v.version, to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at,
 coalesce(t.name, a.name) AS title, a.name AS session_name, c.name AS class_name, sch.name_en, sch.name_bn, sch.logo_url
 FROM result_verifications rv JOIN result_versions v ON v.id = rv.version_id
 JOIN academic_sessions a ON a.id = v.session_id JOIN academic_classes c ON c.id = v.class_id
 LEFT JOIN exam_terms t ON v.scope = 'term' AND t.id = v.scope_id
 CROSS JOIN school_settings sch WHERE sch.id = 1 AND rv.token = ?`, token).Row().
		Scan(&status, &v.Scope, &v.Version, &v.PublishedAt, &v.Title, &v.SessionName, &v.ClassName, &v.SchoolNameEN, &v.SchoolNameBN, &v.LogoURL)
	if errors.Is(err, sql.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, err
	}
	switch status {
	case "published":
		v.State = "current"
	case "superseded":
		v.State = "superseded"
	default:
		v.State = "withdrawn"
	}
	v.Code = documentCode(token)
	return v, nil
}
