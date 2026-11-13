package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

func phoneTestService(t *testing.T) (context.Context, *gorm.DB, *Service) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
	service, err := NewService(NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return ctx, db, service
}

// A guardian's number opens whichever child's account the password belongs
// to, siblings may share it, and the header's switch moves between those
// children only.
func TestPostgresPhoneLoginAcrossSiblings(t *testing.T) {
	ctx, db, service := phoneTestService(t)
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	phone := "+88017" + stamp[len(stamp)-8:]
	if _, ok := NormalizePhone(phone); !ok {
		t.Fatalf("the test number %q is not a valid one", phone)
	}
	other := "+88018" + stamp[len(stamp)-8:]

	student := func(name, password, phone string) string {
		t.Helper()
		hash, err := HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		var id string
		var phoneValue any
		if phone != "" {
			phoneValue = phone
		}
		if err := db.Raw(`INSERT INTO users (email,display_name,role,is_active,password_hash,login_id,login_phone)
 VALUES (?,?,'student',true,?,?,?) RETURNING id`,
			name+"-"+stamp+"@"+PlaceholderDomain, name, hash, name+"-"+stamp, phoneValue).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	const elderPass, youngerPass = "ElderChildPassword1!", "YoungerChildPassword1!"
	elder := student("Elder", elderPass, phone)
	younger := student("Younger", youngerPass, phone)
	stranger := student("Stranger", "StrangerPassword1!", other)

	// The number plus each child's own password opens that child's account.
	for _, c := range []struct {
		password, want string
	}{{elderPass, elder}, {youngerPass, younger}} {
		got, err := service.Login(ctx, FormatPhone(phone), c.password)
		if err != nil {
			t.Fatalf("sign in with the shared number: %v", err)
		}
		if got.User.ID != c.want {
			t.Fatalf("signed in to %s, want %s", got.User.ID, c.want)
		}
	}
	// Every written form of the number works.
	local := "0" + phone[len("+880"):]
	for _, form := range []string{local, "880" + phone[len("+880"):], phone, local[:5] + "-" + local[5:]} {
		if _, err := service.Login(ctx, form, elderPass); err != nil {
			t.Fatalf("sign in with %q: %v", form, err)
		}
	}
	// A wrong password and an unknown number give the very same error.
	unknownErr := loginError(t, service, ctx, "01999000000", elderPass)
	wrongErr := loginError(t, service, ctx, local, "NotTheRightPassword1!")
	if !errors.Is(unknownErr, ErrInvalidCredentials) || !errors.Is(wrongErr, ErrInvalidCredentials) || unknownErr.Error() != wrongErr.Error() {
		t.Fatalf("unknown number %v vs wrong password %v", unknownErr, wrongErr)
	}
	// Another family's password never opens this family's account.
	if err := loginError(t, service, ctx, local, "StrangerPassword1!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a stranger's password on this number: %v", err)
	}

	// The switch lists both children and moves between them without a password.
	list, err := service.Siblings(ctx, elder)
	if err != nil || len(list) != 2 {
		t.Fatalf("siblings: %+v %v", list, err)
	}
	current := 0
	for _, s := range list {
		if s.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("exactly one sibling is the current account: %+v", list)
	}
	if _, err := service.SwitchTo(ctx, elder, younger); err != nil {
		t.Fatalf("switch to a sibling: %v", err)
	}
	// Not to a child of another family, and not to itself from one.
	if _, err := service.SwitchTo(ctx, elder, stranger); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("switch to another family: %v", err)
	}
	if _, err := service.SwitchTo(ctx, stranger, elder); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("switch from another family: %v", err)
	}
	// An only child is offered no switch at all.
	only, err := service.Siblings(ctx, stranger)
	if err != nil || len(only) != 0 {
		t.Fatalf("an only child's siblings: %+v %v", only, err)
	}
}

// Guessing against a shared number counts against every account it could
// open, so the lockout protects siblings the same way it protects an email.
func TestPostgresPhoneLoginLocksOut(t *testing.T) {
	ctx, db, service := phoneTestService(t)
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	phone := "+88019" + stamp[len(stamp)-8:]
	hash, err := HashPassword("LockoutChildPassword1!")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.Raw(`INSERT INTO users (email,display_name,role,is_active,password_hash,login_id,login_phone)
 VALUES (?, 'Lockout', 'student', true, ?, ?, ?) RETURNING id`,
		"lockout-"+stamp+"@"+PlaceholderDomain, hash, "lockout-"+stamp, phone).Row().Scan(&id); err != nil {
		t.Fatal(err)
	}
	local := "0" + phone[len("+880"):]
	for i := 0; i < MaxLoginFailure; i++ {
		if err := loginError(t, service, ctx, local, "WrongPassword1!"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	var attempts int
	var locked *time.Time
	if err := db.Raw("SELECT failed_login_attempts, locked_until FROM users WHERE id = ?", id).Row().Scan(&attempts, &locked); err != nil {
		t.Fatal(err)
	}
	if attempts < MaxLoginFailure || locked == nil || !locked.After(time.Now().UTC()) {
		t.Fatalf("after %d wrong passwords: %d attempts, locked_until %v", MaxLoginFailure, attempts, locked)
	}
	// The right password is refused while the lock holds, with the same error.
	if err := loginError(t, service, ctx, local, "LockoutChildPassword1!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a locked account: %v", err)
	}
}

func loginError(t *testing.T, s *Service, ctx context.Context, identifier, password string) error {
	t.Helper()
	_, err := s.Login(ctx, identifier, password)
	if err == nil {
		t.Fatalf("signing in with %q unexpectedly worked", identifier)
	}
	return err
}
