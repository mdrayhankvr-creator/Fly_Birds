package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/mailer"
)

var (
	// ErrResetTokenInvalid covers a link that is unknown, already used or
	// past its expiry. They are one error on purpose: the page must not say
	// which, or a guessed link tells the guesser how close it was.
	ErrResetTokenInvalid = errors.New("this password reset link is not valid")
	// ErrResetRateLimited means too many links were asked for recently. It
	// counts attempts for addresses that have no account too, so answering
	// with it reveals nothing about which addresses exist.
	ErrResetRateLimited = errors.New("too many password reset requests")
	// ErrResetUnavailable means no mail relay is configured.
	ErrResetUnavailable = errors.New("password reset email is not configured")
)

const (
	// ResetLinkLifetime is how long an emailed link works. Long enough for
	// someone to find the message on a phone, short enough that a forwarded
	// or quoted email stops being a key by the end of a lesson.
	ResetLinkLifetime = time.Hour
	// ResetRateWindow is the period the two limits below are counted over.
	ResetRateWindow = time.Hour
	// ResetMaxPerEmail bounds requests for one address: enough for a mistyped
	// attempt and a retry, not enough to flood somebody's inbox.
	ResetMaxPerEmail = 3
	// ResetMaxPerIP bounds requests from one client, so a script cannot walk
	// a list of addresses. A whole school behind one connection is well
	// inside it, because a reset is rare.
	ResetMaxPerIP = 20
)

// ResetAttempts counts recent requests for the rate limit.
type ResetAttempts struct{ ByEmail, ByIP int64 }

// ResetRepository stores the reset links and the attempt log. It is separate
// from Repository so that the sign-in path and its test doubles are
// unaffected by a feature that is off unless a relay is configured.
type ResetRepository interface {
	// FindResetCandidate returns the one account that may be recovered with
	// this address, or ErrNotFound. An inactive account, or one whose email
	// is the placeholder a student sign-in gets, is not a candidate.
	FindResetCandidate(ctx context.Context, email string) (User, error)
	// RecentResetAttempts counts the requests logged for this address and
	// from this client since the given time.
	RecentResetAttempts(ctx context.Context, emailDigest []byte, ip string, since time.Time) (ResetAttempts, error)
	// LogResetAttempt records a request that issued no link, so that an
	// address with no account still counts toward the limits.
	LogResetAttempt(ctx context.Context, emailDigest []byte, ip string, now time.Time) error
	// CreateResetToken records an issued link.
	CreateResetToken(ctx context.Context, emailDigest []byte, ip, userID string, tokenDigest []byte, now, expiresAt time.Time) error
	// ResetTokenUser returns the account a live link belongs to without
	// using the link up.
	ResetTokenUser(ctx context.Context, tokenDigest []byte, now time.Time) (User, error)
	// ConsumeResetToken sets the password through a live link in one
	// transaction: the link and the account's other links are marked used,
	// every session is revoked and the change is audited. It returns the
	// account and how many sessions were signed out.
	ConsumeResetToken(ctx context.Context, tokenDigest []byte, newHash string, now time.Time) (User, int64, error)
}

// resetMail is everything the service needs to send a link.
type resetMail struct {
	repo ResetRepository
	send mailer.Mailer
	// baseURL is the school's public address; the link is baseURL plus the
	// reset page and the token.
	baseURL string
	// names returns the school's English and Bangla names for the message.
	// A nil hook falls back to the address of the site.
	names  func(context.Context) (string, string)
	logger *slog.Logger
}

// EnablePasswordResetEmail switches on "forgot password". Without it, and so
// without a mail relay, the sign-in page keeps telling people to ask the
// school office and an administrator resets passwords as before.
//
// baseURL must be the school's public address (PUBLIC_BASE_URL): it is what
// the emailed link points at.
func (s *Service) EnablePasswordResetEmail(repo ResetRepository, send mailer.Mailer, baseURL string, names func(context.Context) (string, string), logger *slog.Logger) error {
	if repo == nil || send == nil {
		return errors.New("password reset email needs a store and a mail relay")
	}
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("password reset email needs the school's public address, such as https://school.example.edu.bd")
	}
	if logger == nil {
		logger = slog.Default()
	}
	s.reset = &resetMail{repo: repo, send: send, baseURL: trimmed, names: names, logger: logger}
	return nil
}

// PasswordResetEmailEnabled reports whether the sign-in page should offer
// "forgot password".
func (s *Service) PasswordResetEmailEnabled() bool { return s != nil && s.reset != nil }

// emailDigest identifies an address in the attempt log without storing it,
// so the log cannot be mined for the school's addresses.
func emailDigest(email string) []byte {
	sum := sha256.Sum256([]byte(email))
	return sum[:]
}

// RequestPasswordReset is the "forgot password" form. It answers the same way
// whether or not the address has an account: only the mailbox owner learns
// anything. The mail is handed to the relay in the background, so the time
// the request takes does not say whether a message was sent.
//
// It returns ErrResetRateLimited when the address or the client has asked too
// often, and ErrResetUnavailable when no relay is configured.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail, ip, language string) error {
	if !s.PasswordResetEmailEnabled() {
		return ErrResetUnavailable
	}
	reset := s.reset
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		// Not an address at all: nothing to look up and nothing to count.
		return nil
	}
	now := s.now()
	digest := emailDigest(email)
	attempts, err := reset.repo.RecentResetAttempts(ctx, digest, ip, now.Add(-ResetRateWindow))
	if err != nil {
		return err
	}
	if attempts.ByEmail >= ResetMaxPerEmail || (ip != "" && attempts.ByIP >= ResetMaxPerIP) {
		return ErrResetRateLimited
	}
	user, err := reset.repo.FindResetCandidate(ctx, email)
	if errors.Is(err, ErrNotFound) {
		// Logged without a link, so the next attempt for this address is
		// still counted. The caller gets the same answer as a hit.
		return reset.repo.LogResetAttempt(ctx, digest, ip, now)
	}
	if err != nil {
		return err
	}
	token, err := randomOpaque()
	if err != nil {
		return err
	}
	tokenDigest, err := hashOpaque(token)
	if err != nil {
		return err
	}
	expires := now.Add(ResetLinkLifetime)
	if err := reset.repo.CreateResetToken(ctx, digest, ip, user.ID, tokenDigest, now, expires); err != nil {
		return err
	}
	go reset.deliver(user, token, language, expires)
	return nil
}

// deliver sends the message outside the request. A failure is logged with the
// account id, never the address or the link.
func (r *resetMail) deliver(user User, token, language string, expires time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nameEN, nameBN := "", ""
	if r.names != nil {
		nameEN, nameBN = r.names(ctx)
	}
	message := resetMessage(resetMailContent{
		SchoolEN: nameEN, SchoolBN: nameBN, Site: r.baseURL,
		To: user.Email, DisplayName: user.DisplayName,
		Link: r.link(token), Language: language, Minutes: int(ResetLinkLifetime / time.Minute),
	})
	if err := r.send.Send(ctx, message); err != nil {
		r.logger.Error("password reset email failed", "user_id", user.ID, "error", err)
		return
	}
	r.logger.Info("password reset email sent", "user_id", user.ID, "expires_at", expires.Format(time.RFC3339))
}

// link is the address in the message. The token travels in the query string
// of the school's own reset page.
func (r *resetMail) link(token string) string {
	return r.baseURL + "/reset-password?token=" + url.QueryEscape(token)
}

// PasswordResetTarget reports whose account a link belongs to, without using
// the link up, so the page can greet the right person before the new password
// is typed.
func (s *Service) PasswordResetTarget(ctx context.Context, token string) (User, error) {
	if !s.PasswordResetEmailEnabled() {
		return User{}, ErrResetUnavailable
	}
	digest, err := hashOpaque(token)
	if err != nil {
		return User{}, ErrResetTokenInvalid
	}
	user, err := s.reset.repo.ResetTokenUser(ctx, digest, s.now())
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrResetTokenInvalid
	}
	return user, err
}

// CompletePasswordReset sets a new password through an emailed link and
// returns how many sessions were signed out. The link works once: using it
// marks it, and the account's other outstanding links, used.
func (s *Service) CompletePasswordReset(ctx context.Context, token, password string) (User, int64, error) {
	if !s.PasswordResetEmailEnabled() {
		return User{}, 0, ErrResetUnavailable
	}
	if !utf8.ValidString(password) || len(password) < 12 || len(password) > 72 {
		return User{}, 0, ErrWeakPassword
	}
	digest, err := hashOpaque(token)
	if err != nil {
		return User{}, 0, ErrResetTokenInvalid
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, 0, err
	}
	user, revoked, err := s.reset.repo.ConsumeResetToken(ctx, digest, hash, s.now())
	if errors.Is(err, ErrNotFound) {
		return User{}, 0, ErrResetTokenInvalid
	}
	return user, revoked, err
}

// resetMailContent is what the message is built from.
type resetMailContent struct {
	SchoolEN, SchoolBN string
	Site               string
	To, DisplayName    string
	Link               string
	Language           string
	Minutes            int
}

// school names the school in the reader's language, falling back to the other
// name and then to the site's address.
func (c resetMailContent) school(bangla bool) string {
	order := []string{c.SchoolEN, c.SchoolBN}
	if bangla {
		order = []string{c.SchoolBN, c.SchoolEN}
	}
	for _, name := range order {
		if strings.TrimSpace(name) != "" {
			return name
		}
	}
	if parsed, err := url.Parse(c.Site); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return c.Site
}

// resetMessage writes the email. It is bilingual, with the reader's own
// language first, because a school's staff and families use both.
func resetMessage(c resetMailContent) mailer.Message {
	bangla := c.Language == "bn"
	en := resetParagraphs{
		subject: fmt.Sprintf("Reset your %s password", c.school(false)),
		greet:   greeting("Hello", c.DisplayName),
		lead:    fmt.Sprintf("Someone asked to reset the password for your %s account. Open the link below to choose a new one.", c.school(false)),
		button:  "Choose a new password",
		expiry:  fmt.Sprintf("The link works once and stops working in %d minutes.", c.Minutes),
		ignore:  "If you did not ask for this, ignore this email. Your password has not changed. If it keeps happening, tell the school office.",
	}
	bn := resetParagraphs{
		subject: fmt.Sprintf("%s-এর পাসওয়ার্ড নতুন করে দিন", c.school(true)),
		greet:   greeting("হ্যালো", c.DisplayName),
		lead:    fmt.Sprintf("আপনার %s অ্যাকাউন্টের পাসওয়ার্ড নতুন করে দেওয়ার অনুরোধ এসেছে। নিচের লিংকে গিয়ে নতুন পাসওয়ার্ড দিন।", c.school(true)),
		button:  "নতুন পাসওয়ার্ড দিন",
		expiry:  fmt.Sprintf("লিংকটি একবারই কাজ করবে এবং %d মিনিট পরে আর কাজ করবে না।", c.Minutes),
		ignore:  "আপনি অনুরোধ না করলে এই ইমেইলটি বাদ দিন; আপনার পাসওয়ার্ড বদলায়নি। বারবার এমন হলে স্কুল অফিসে জানান।",
	}
	first, second := en, bn
	if bangla {
		first, second = bn, en
	}
	return mailer.Message{
		To:      c.To,
		Subject: first.subject,
		Text:    resetText(first, second, c.Link),
		HTML:    resetHTML(first, second, c.Link),
	}
}

type resetParagraphs struct{ subject, greet, lead, button, expiry, ignore string }

func greeting(hello, name string) string {
	if strings.TrimSpace(name) == "" {
		return hello
	}
	return hello + " " + strings.TrimSpace(name)
}

func resetText(first, second resetParagraphs, link string) string {
	var out strings.Builder
	for i, p := range []resetParagraphs{first, second} {
		if i > 0 {
			out.WriteString("\n----------------------------------------\n\n")
		}
		out.WriteString(p.greet + ",\n\n" + p.lead + "\n\n" + link + "\n\n" + p.expiry + "\n\n" + p.ignore + "\n")
	}
	return out.String()
}

func resetHTML(first, second resetParagraphs, link string) string {
	var out strings.Builder
	out.WriteString(`<!doctype html><html><body style="margin:0;background:#f8fafc;font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;color:#0f172a">`)
	out.WriteString(`<div style="max-width:560px;margin:0 auto;padding:24px">`)
	for i, p := range []resetParagraphs{first, second} {
		if i > 0 {
			out.WriteString(`<hr style="margin:28px 0;border:none;border-top:1px solid #e2e8f0">`)
		}
		out.WriteString(`<p style="margin:0 0 12px;font-size:16px">` + escapeHTML(p.greet) + `,</p>`)
		out.WriteString(`<p style="margin:0 0 20px;font-size:15px;line-height:1.6">` + escapeHTML(p.lead) + `</p>`)
		out.WriteString(`<p style="margin:0 0 20px"><a href="` + escapeHTML(link) + `" style="display:inline-block;padding:12px 20px;border-radius:10px;background:#1d4ed8;color:#ffffff;font-weight:600;font-size:15px;text-decoration:none">` + escapeHTML(p.button) + `</a></p>`)
		out.WriteString(`<p style="margin:0 0 12px;font-size:13px;color:#475569;word-break:break-all">` + escapeHTML(link) + `</p>`)
		out.WriteString(`<p style="margin:0 0 8px;font-size:13px;color:#475569">` + escapeHTML(p.expiry) + `</p>`)
		out.WriteString(`<p style="margin:0;font-size:13px;color:#475569">` + escapeHTML(p.ignore) + `</p>`)
	}
	out.WriteString(`</div></body></html>`)
	return out.String()
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(value)
}
