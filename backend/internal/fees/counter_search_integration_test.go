package fees

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// TestPostgresCounterSearch: at the fee counter a student is found by
// admission number, name, roll number or the guardian's phone, typed in any
// format and in Bangla digits.
func TestPostgresCounterSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newFeeFixture(t, ctx, "2182")
	svc := NewService(NewRepository(f.db), nil)
	// The fixture's admission numbers are "M8-<stamp>-n", and a typed phone
	// number also matches admission numbers that contain it. The phone must
	// therefore never occur in the stamp, and its last six digits must not
	// read as a small roll number.
	phone := counterTestPhone(f.stamp, rand.IntN)
	if err := f.db.Exec("UPDATE guardians SET phone=? WHERE id=(SELECT guardian_id FROM student_guardians WHERE student_id=?)", "+880 "+phone[1:5]+"-"+phone[5:], f.second).Error; err != nil {
		t.Fatal(err)
	}
	find := func(search string) []string {
		t.Helper()
		page, err := svc.SearchStudents(ctx, Query{Search: search, SessionID: f.session, Page: 1, PageSize: 20})
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, s := range page.Items {
			ids = append(ids, s.ID)
		}
		return ids
	}
	only := func(label string, got []string, want string) {
		t.Helper()
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: got %v, want only %s", label, got, want)
		}
	}
	bangla := strings.NewReplacer("0", "০", "1", "১", "2", "২", "3", "৩", "4", "৪", "5", "৫", "6", "৬", "7", "৭", "8", "৮", "9", "৯")
	first := func(label string, got []string, want string) {
		t.Helper()
		if len(got) == 0 || got[0] != want {
			t.Errorf("%s: got %v, want %s first", label, got, want)
		}
	}
	first("roll number", find("2"), f.second)
	first("roll number in Bangla digits", find("২"), f.second)
	only("guardian phone", find(phone), f.second)
	only("guardian phone with country code and spaces", find("+880 "+phone[1:]), f.second)
	only("last digits of the guardian phone in Bangla", find(bangla.Replace(phone[len(phone)-6:])), f.second)
	only("admission number", find("M8-"+f.stamp+"-1"), f.first)
	if got := find("Fee Student"); len(got) != 3 {
		t.Errorf("name search: got %d students, want 3", len(got))
	}
	if got := find("99999"); len(got) != 0 {
		t.Errorf("unknown number matched %v", got)
	}
}

// counterTestPhone returns a mobile number "019xxxxxxxx" that does not occur
// in stamp and whose last six digits do not start with 0.
func counterTestPhone(stamp string, intN func(int) int) string {
	for {
		phone := fmt.Sprintf("019%08d", intN(100_000_000))
		if !strings.Contains(stamp, phone) && phone[len(phone)-6] != '0' {
			return phone
		}
	}
}

// The phone avoids the stamp even when the first choices collide with it.
func TestCounterTestPhoneAvoidsTheStamp(t *testing.T) {
	stamp := "1793801912345678000"
	picks := []int{12345678, 99012345, 23456789} // 01912345678 is in the stamp; 01999012345 has 0 six from the end
	i := 0
	phone := counterTestPhone(stamp, func(int) int { i++; return picks[i-1] })
	if phone != "01923456789" || i != 3 {
		t.Fatalf("got %s after %d picks", phone, i)
	}
}
