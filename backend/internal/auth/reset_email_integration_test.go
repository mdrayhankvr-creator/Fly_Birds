package auth

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/mailer"
	"gorm.io/gorm"
)

// captureMailer stands in for the SMTP relay and hands each message to the
// test, so the test reads the link exactly as the recipient would.
type captureMailer struct {
	sent chan mailer.Message
	fail error
}

func newCaptureMailer() *captureMailer {
	return &captureMailer{sent: make(chan mailer.Message, 8)}
}

func (m *captureMailer) Send(_ context.Context, message mailer.Message) error {
	if m.fail != nil {
		return m.fail
	}
	m.sent <- message
	return nil
}

// next waits for the message the service sends in the background.
func (m *captureMailer) next(t *testing.T) mailer.Message {
	t.Helper()
	select {
	case message := <-m.sent:
		return message
	case <-time.After(10 * time.Second):
		t.Fatal("no password reset email was sent")
		return mailer.Message{}
	}
}

// silent reports that nothing was sent. A short wait is enough: the send is
// started before RequestPasswordReset returns.
func (m *captureMailer) silent(t *testing.T) {
	t.Helper()
	select {
	case message := <-m.sent:
		t.Fatalf("an email was sent when none should have been: %q", message.Subject)
	case <-time.After(300 * time.Millisecond):
	}
}

// linkToken pulls the token out of the link in the message body.
func linkToken(t *testing.T, message mailer.Message) string {
	t.Helper()
	marker := "/reset-password?token="
	index := strings.Index(message.Text, marker)
	if index < 0 {
		t.Fatalf("no reset link in the message body: %q", message.Text)
	}
	raw := message.Text[index+len(marker):]
	if cut := strings.IndexAny(raw, " \r\n"); cut >= 0 {
		raw = raw[:cut]
	}
	token, err := url.QueryUnescape(raw)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// resetFixture is one account with the email reset flow switched on and the
// clock under the test's control.
type resetFixture struct {
	db      *gorm.DB
	service *Service
	repo    *GormRepository
	mail    *captureMailer
	email   string
	// ip is unique to this fixture, so repeated runs against the same test
	// database never share the per-client bucket.
	ip   string
	user User
	now  time.Time
}

func newResetFixture(t *testing.T, ctx context.Context, db *gorm.DB) *resetFixture {
	t.Helper()
	random, _ := randomOpaque()
	suffix := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(random))[:16]
	fixture := &resetFixture{db: db, repo: NewRepository(db), mail: newCaptureMailer(), now: time.Now().UTC().Truncate(time.Second)}
	service, err := NewService(fixture.repo, bytes.Repeat([]byte{0x31}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return fixture.now }
	if err := service.EnablePasswordResetEmail(fixture.repo, fixture.mail, "https://school.example.test/", func(context.Context) (string, string) {
		return "Jalalabad High School", "জালালাবাদ উচ্চ বিদ্যালয়"
	}, nil); err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	fixture.email = "reset-" + suffix + "@example.test"
	fixture.ip = "2001:db8::" + suffix
	user, err := service.BootstrapUser(ctx, fixture.email, "OriginalPassword!123", "teacher")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&User{}).Where("id = ?", user.ID).Update("display_name", "Rumi Akter").Error; err != nil {
		t.Fatal(err)
	}
	user.DisplayName = "Rumi Akter"
	fixture.user = user
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM password_reset_requests WHERE requester_ip = ?", fixture.ip).Error
		_ = db.Exec("DELETE FROM audit_events WHERE target_id = ?", user.ID).Error
		_ = db.Where("id = ?", user.ID).Delete(&User{}).Error
	})
	return fixture
}

// request asks for a link and returns the token from the email.
func (f *resetFixture) request(t *testing.T, ctx context.Context, email, ip, language string) string {
	t.Helper()
	if err := f.service.RequestPasswordReset(ctx, email, ip, language); err != nil {
		t.Fatalf("request a reset link: %v", err)
	}
	return linkToken(t, f.mail.next(t))
}

func openResetDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return ctx, db
}

// TestPostgresPasswordResetLink is the happy path: the emailed link names the
// school in both languages, works once, clears a lockout and signs every
// session out.
func TestPostgresPasswordResetLink(t *testing.T) {
	ctx, db := openResetDB(t)
	f := newResetFixture(t, ctx, db)

	// A session and a lockout to be cleared by the reset.
	before, err := f.service.Login(ctx, f.email, "OriginalPassword!123")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxLoginFailure; i++ {
		_, _ = f.service.Login(ctx, f.email, "wrong-password")
	}
	if _, err := f.service.Login(ctx, f.email, "OriginalPassword!123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the account should be locked out: %v", err)
	}

	if err := f.service.RequestPasswordReset(ctx, "  "+strings.ToUpper(f.email)+"  ", f.ip, "bn"); err != nil {
		t.Fatalf("request a reset link: %v", err)
	}
	message := f.mail.next(t)
	if message.To != f.email {
		t.Fatalf("addressed to %q", message.To)
	}
	// Bangla first for a Bangla reader, and both languages present.
	if !strings.Contains(message.Subject, "জালালাবাদ উচ্চ বিদ্যালয়") {
		t.Fatalf("subject is not the Bangla one: %q", message.Subject)
	}
	for _, want := range []string{"জালালাবাদ উচ্চ বিদ্যালয়", "Jalalabad High School", "60 minutes", "60 মিনিট", "Rumi Akter"} {
		if !strings.Contains(message.Text, want) {
			t.Fatalf("the message does not mention %q", want)
		}
	}
	if strings.Contains(message.HTML, "<script") {
		t.Fatal("the HTML body is not escaped")
	}
	token := linkToken(t, message)

	// The form checks the link before asking for a password.
	target, err := f.service.PasswordResetTarget(ctx, token)
	if err != nil || target.ID != f.user.ID || target.DisplayName != "Rumi Akter" {
		t.Fatalf("check the link: %+v %v", target, err)
	}

	user, revoked, err := f.service.CompletePasswordReset(ctx, token, "ReplacementPassword!456")
	if err != nil {
		t.Fatalf("set the password through the link: %v", err)
	}
	if user.ID != f.user.ID || revoked != 1 {
		t.Fatalf("reset returned %s with %d session(s) revoked", user.ID, revoked)
	}
	if _, err := f.service.Authenticate(ctx, before.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("the old access token still works: %v", err)
	}
	if _, err := f.service.Login(ctx, f.email, "OriginalPassword!123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the old password still works: %v", err)
	}
	// The lockout is gone and the account owns its password, so there is no
	// forced change on the next sign-in.
	credentials, err := f.service.Login(ctx, f.email, "ReplacementPassword!456")
	if err != nil {
		t.Fatalf("the new password was refused (lockout not cleared?): %v", err)
	}
	if credentials.User.MustChangePassword {
		t.Fatal("a password the account chose itself should not force a change")
	}
	var audited int64
	db.Table("audit_events").Where("target_id = ? AND action IN ('user.password_reset_requested','user.password_reset')", f.user.ID).Count(&audited)
	if audited != 2 {
		t.Fatalf("the request and the reset should be audited once each; found %d records", audited)
	}
}

// TestPostgresPasswordResetLinkIsSingleUse covers the three ways a link stops
// working: it has been used, it has expired, or it was never issued.
func TestPostgresPasswordResetLinkIsSingleUse(t *testing.T) {
	ctx, db := openResetDB(t)
	f := newResetFixture(t, ctx, db)

	token := f.request(t, ctx, f.email, f.ip, "en")
	if _, _, err := f.service.CompletePasswordReset(ctx, token, "FirstReplacement!123"); err != nil {
		t.Fatal(err)
	}
	// Used once, and that is the end of it.
	if _, _, err := f.service.CompletePasswordReset(ctx, token, "SecondReplacement!123"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("the link worked a second time: %v", err)
	}
	if _, err := f.service.PasswordResetTarget(ctx, token); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("a used link still passes the check: %v", err)
	}
	if _, err := f.service.Login(ctx, f.email, "SecondReplacement!123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the second password was accepted: %v", err)
	}
	if _, err := f.service.Login(ctx, f.email, "FirstReplacement!123"); err != nil {
		t.Fatalf("the first password should still work: %v", err)
	}

	// Expiry: a link is dead an hour after it was issued, to the second.
	issued := f.now
	expiring := f.request(t, ctx, f.email, f.ip, "en")
	f.now = issued.Add(ResetLinkLifetime - time.Second)
	if _, err := f.service.PasswordResetTarget(ctx, expiring); err != nil {
		t.Fatalf("the link should still work just before it expires: %v", err)
	}
	f.now = issued.Add(ResetLinkLifetime)
	if _, err := f.service.PasswordResetTarget(ctx, expiring); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("an expired link still passes the check: %v", err)
	}
	if _, _, err := f.service.CompletePasswordReset(ctx, expiring, "ThirdReplacement!123"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("an expired link set the password: %v", err)
	}

	// A token that was never issued, and a string that is not a token at all.
	unknown, _ := randomOpaque()
	for _, bad := range []string{unknown, "not-a-token", ""} {
		if _, _, err := f.service.CompletePasswordReset(ctx, bad, "FourthReplacement!123"); !errors.Is(err, ErrResetTokenInvalid) {
			t.Fatalf("token %q: %v", bad, err)
		}
	}
	// A short password is refused before the link is touched, so the link
	// survives a mistyped attempt.
	live := f.request(t, ctx, f.email, f.ip, "en")
	if _, _, err := f.service.CompletePasswordReset(ctx, live, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("a short password: %v", err)
	}
	if _, err := f.service.PasswordResetTarget(ctx, live); err != nil {
		t.Fatalf("the link should survive a refused password: %v", err)
	}
}

// TestPostgresPasswordResetOlderLinksDie checks that using one link kills the
// account's other outstanding links, so an older email in the same inbox is
// not a second key.
func TestPostgresPasswordResetOlderLinksDie(t *testing.T) {
	ctx, db := openResetDB(t)
	f := newResetFixture(t, ctx, db)

	first := f.request(t, ctx, f.email, f.ip, "en")
	second := f.request(t, ctx, f.email, f.ip, "en")
	if first == second {
		t.Fatal("two requests produced the same token")
	}
	if _, _, err := f.service.CompletePasswordReset(ctx, second, "ReplacementPassword!456"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.CompletePasswordReset(ctx, first, "AnotherReplacement!789"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("the earlier link still works: %v", err)
	}
	var live int64
	db.Model(&ResetRequest{}).Where("user_id = ? AND used_at IS NULL AND token_sha256 IS NOT NULL", f.user.ID).Count(&live)
	if live != 0 {
		t.Fatalf("%d link(s) left outstanding", live)
	}
}

// TestPostgresPasswordResetUnknownAddress checks that an address with no
// account, a suspended account and a student sign-in without a real mailbox
// all answer the same way as a success and send nothing.
func TestPostgresPasswordResetUnknownAddress(t *testing.T) {
	ctx, db := openResetDB(t)
	f := newResetFixture(t, ctx, db)

	random, _ := randomOpaque()
	unknown := "nobody-" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(random))[:12] + "@example.test"
	if err := f.service.RequestPasswordReset(ctx, unknown, f.ip, "en"); err != nil {
		t.Fatalf("an unknown address should be accepted silently: %v", err)
	}
	f.mail.silent(t)
	// Logged all the same, so the next attempt for it is counted.
	var logged int64
	db.Model(&ResetRequest{}).Where("email_sha256 = ?", emailDigest(unknown)).Count(&logged)
	if logged != 1 {
		t.Fatalf("an unknown address logged %d attempts", logged)
	}
	var tokens int64
	db.Model(&ResetRequest{}).Where("email_sha256 = ? AND token_sha256 IS NOT NULL", emailDigest(unknown)).Count(&tokens)
	if tokens != 0 {
		t.Fatalf("a link was issued for an address with no account (%d)", tokens)
	}

	// A suspended account is not recovered by email.
	if err := db.Model(&User{}).Where("id = ?", f.user.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.RequestPasswordReset(ctx, f.email, f.ip, "en"); err != nil {
		t.Fatal(err)
	}
	f.mail.silent(t)
	if err := db.Model(&User{}).Where("id = ?", f.user.ID).Update("is_active", true).Error; err != nil {
		t.Fatal(err)
	}

	// A student sign-in has a placeholder address with no mailbox behind it.
	placeholder := "student-" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(random))[:10] + "@" + PlaceholderDomain
	student, err := f.service.BootstrapUser(ctx, placeholder, "StudentPassword!123", "student")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Where("id = ?", student.ID).Delete(&User{}).Error })
	if err := f.service.RequestPasswordReset(ctx, placeholder, f.ip, "en"); err != nil {
		t.Fatal(err)
	}
	f.mail.silent(t)

	// Something that is not an address at all is simply ignored.
	if err := f.service.RequestPasswordReset(ctx, "not-an-address", f.ip, "en"); err != nil {
		t.Fatal(err)
	}
	f.mail.silent(t)
}

// TestPostgresPasswordResetRateLimit checks both limits, and that the limit on
// an address counts attempts for addresses with no account too, so it cannot
// be used to find out which addresses exist.
func TestPostgresPasswordResetRateLimit(t *testing.T) {
	ctx, db := openResetDB(t)
	f := newResetFixture(t, ctx, db)

	for i := 0; i < ResetMaxPerEmail; i++ {
		f.request(t, ctx, f.email, f.ip, "en")
	}
	if err := f.service.RequestPasswordReset(ctx, f.email, f.ip, "en"); !errors.Is(err, ErrResetRateLimited) {
		t.Fatalf("the limit for one address did not hold: %v", err)
	}
	f.mail.silent(t)
	// The window moves: an hour on, the address may ask again.
	f.now = f.now.Add(ResetRateWindow + time.Second)
	f.request(t, ctx, f.email, f.ip, "en")

	// An address with no account is counted the same way, so hitting the
	// limit says nothing about whether the address exists.
	random, _ := randomOpaque()
	unknown := "nobody-" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(random))[:12] + "@example.test"
	for i := 0; i < ResetMaxPerEmail; i++ {
		if err := f.service.RequestPasswordReset(ctx, unknown, f.ip, "en"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.service.RequestPasswordReset(ctx, unknown, f.ip, "en"); !errors.Is(err, ErrResetRateLimited) {
		t.Fatalf("an address with no account escapes the limit: %v", err)
	}

	// The limit per client stops one machine walking a list of addresses.
	ip := f.ip + ":walk"
	var sent int
	for i := 0; i < ResetMaxPerIP+2; i++ {
		other, _ := randomOpaque()
		address := "walk-" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(other))[:12] + "@example.test"
		err := f.service.RequestPasswordReset(ctx, address, ip, "en")
		if errors.Is(err, ErrResetRateLimited) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		sent++
	}
	if sent != ResetMaxPerIP {
		t.Fatalf("the limit for one client allowed %d requests, expected %d", sent, ResetMaxPerIP)
	}
	t.Cleanup(func() { _ = db.Where("requester_ip = ?", ip).Delete(&ResetRequest{}).Error })

	// A relay that will not take the message is not the caller's problem:
	// the request still succeeds, so the answer stays the same either way.
	f.now = f.now.Add(2 * ResetRateWindow)
	f.mail.fail = errors.New("relay refused")
	if err := f.service.RequestPasswordReset(ctx, f.email, f.ip, "en"); err != nil {
		t.Fatalf("a failing relay changed the answer: %v", err)
	}
}
