package fees

import (
	"context"
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
	phone := "019" + f.stamp[len(f.stamp)-8:]
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
