package fees

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestToCentsRejectsUnsafeAmounts(t *testing.T) {
	for _, v := range []float64{-1, 0.001, 12.345, 100000001} {
		if _, ok := ToCents(v); ok {
			t.Fatalf("%v accepted", v)
		}
	}
	for v, want := range map[float64]int64{0: 0, 0.1: 10, 1500.5: 150050, 99.99: 9999, 0.07: 7} {
		if got, ok := ToCents(v); !ok || got != want {
			t.Fatalf("%v: got %d %v want %d", v, got, ok, want)
		}
	}
	if Decimal(150050) != "1500.50" || Decimal(7) != "0.07" || Decimal(-250) != "-2.50" {
		t.Fatal("decimal rendering")
	}
}

func TestPercentOfRoundsToNearestPoisha(t *testing.T) {
	cases := []struct {
		amount  int64
		percent float64
		want    int64
	}{{150000, 50, 75000}, {150000, 100, 150000}, {33333, 12.5, 4167}, {100, 33.33, 33}, {101, 50, 51}}
	for _, c := range cases {
		if got := PercentOf(c.amount, c.percent); got != c.want {
			t.Fatalf("%d x %v%%: got %d want %d", c.amount, c.percent, got, c.want)
		}
	}
}

func TestAllocateSettlesOldestDuesAndRejectsOverpayment(t *testing.T) {
	dues := []Outstanding{{"jan", 100000}, {"zero", 0}, {"feb", 100000}, {"exam", 50000}}
	got, err := Allocate(150000, dues)
	if err != nil || len(got) != 2 || got[0] != (Split{"jan", 100000}) || got[1] != (Split{"feb", 50000}) {
		t.Fatalf("partial allocation %+v %v", got, err)
	}
	if got, err = Allocate(250000, dues); err != nil || len(got) != 3 || got[2] != (Split{"exam", 50000}) {
		t.Fatalf("full allocation %+v %v", got, err)
	}
	if _, err := Allocate(250001, dues); !errors.Is(err, ErrOverpayment) {
		t.Fatalf("overpayment accepted: %v", err)
	}
	if _, err := Allocate(1, nil); !errors.Is(err, ErrOverpayment) {
		t.Fatalf("payment without dues accepted: %v", err)
	}
	if _, err := Allocate(0, dues); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero payment accepted: %v", err)
	}
}

func TestMockGatewayIssuesReferencesAndApprovesOnlyMockOTP(t *testing.T) {
	g := MockGateway{}
	ref, err := g.Initiate(context.Background(), "bkash", 100)
	if err != nil || len(ref) != 14 || ref[:2] != "BK" {
		t.Fatalf("reference %q %v", ref, err)
	}
	other, _ := g.Initiate(context.Background(), "nagad", 100)
	if other[:2] != "NG" || other == ref {
		t.Fatalf("nagad reference %q", other)
	}
	if _, err := g.Initiate(context.Background(), "rocket", 100); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown provider accepted")
	}
	if err := g.Verify(context.Background(), "bkash", ref, "000000"); !errors.Is(err, ErrDeclined) {
		t.Fatalf("wrong OTP: %v", err)
	}
	if err := g.Verify(context.Background(), "bkash", ref, MockOTP); err != nil {
		t.Fatal(err)
	}
}

// recordingRepo fails the test if validation lets a bad request through.
type recordingRepo struct {
	Repository
	payments []PaymentRecord
	adjusted []int64
}

func (r *recordingRepo) RecordPayment(_ context.Context, _ string, rec PaymentRecord) (string, bool, error) {
	r.payments = append(r.payments, rec)
	return "p", false, nil
}
func (r *recordingRepo) Receipt(context.Context, string, string) (Receipt, error) {
	return Receipt{}, nil
}
func (r *recordingRepo) Adjust(_ context.Context, _, _, _ string, cents int64, _ string) (Due, error) {
	r.adjusted = append(r.adjusted, cents)
	return Due{}, nil
}

const (
	actor   = "11111111-1111-1111-1111-111111111111"
	student = "22222222-2222-2222-2222-222222222222"
	key     = "33333333-3333-3333-3333-333333333333"
)

func TestRecordPaymentValidation(t *testing.T) {
	repo := &recordingRepo{}
	s := NewService(repo, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC) } // 18 Oct in Dhaka
	valid := PaymentInput{StudentID: student, Amount: 1500.5, Method: "cash", IdempotencyKey: key}
	bad := []PaymentInput{
		{StudentID: student, Amount: 0, Method: "cash", IdempotencyKey: key},
		{StudentID: student, Amount: -5, Method: "cash", IdempotencyKey: key},
		{StudentID: student, Amount: 10.555, Method: "cash", IdempotencyKey: key},
		{StudentID: student, Amount: 10, Method: "cheque", IdempotencyKey: key},
		{StudentID: student, Amount: 10, Method: "bkash", IdempotencyKey: key},
		{StudentID: student, Amount: 10, Method: "cash"},
		{StudentID: student, Amount: 10, Method: "cash", IdempotencyKey: key, PaidOn: "2026-10-19"},
		{StudentID: student, Amount: 10, Method: "cash", IdempotencyKey: key, FeeIDs: []string{key, key}},
		{StudentID: "x", Amount: 10, Method: "cash", IdempotencyKey: key},
	}
	for i, in := range bad {
		if _, err := s.RecordPayment(context.Background(), actor, in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	if _, err := s.RecordPayment(context.Background(), actor, valid); err != nil {
		t.Fatal(err)
	}
	got := repo.payments[0]
	if len(repo.payments) != 1 || got.AmountCents != 150050 || got.PaidOn != "2026-10-18" || got.Source != "counter" {
		t.Fatalf("recorded %+v", repo.payments)
	}
}

func TestAdjustRestrictsWaiversToAdministrators(t *testing.T) {
	repo := &recordingRepo{}
	s := NewService(repo, nil)
	due := key
	if _, err := s.Adjust(context.Background(), actor, "accountant", due, AdjustmentInput{Kind: "waiver", Reason: "Hardship"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("accountant waiver: %v", err)
	}
	if _, err := s.Adjust(context.Background(), actor, "accountant", due, AdjustmentInput{Kind: "discount", Amount: 0, Reason: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero discount: %v", err)
	}
	if _, err := s.Adjust(context.Background(), actor, "accountant", due, AdjustmentInput{Kind: "discount", Amount: 200, Reason: " "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank reason: %v", err)
	}
	if _, err := s.Adjust(context.Background(), actor, "accountant", due, AdjustmentInput{Kind: "discount", Amount: 200, Reason: "Sibling"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Adjust(context.Background(), actor, "admin", due, AdjustmentInput{Kind: "waiver", Amount: 999, Reason: "Flood relief"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.adjusted) != 2 || repo.adjusted[0] != 20000 || repo.adjusted[1] != 0 {
		t.Fatalf("adjustments %+v", repo.adjusted)
	}
}

func TestLedgerEntryAndQueryValidation(t *testing.T) {
	s := NewService(&recordingRepo{}, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC) }
	for i, in := range []EntryInput{
		{EntryType: "income", Category: "fees", Amount: 10, EntryDate: "2026-10-01", Description: "Manual fee"},
		{EntryType: "expense", Category: "donation", Amount: 10, EntryDate: "2026-10-01", Description: "Wrong side"},
		{EntryType: "expense", Category: "utilities", Amount: 10, EntryDate: "2026-10-30", Description: "Future"},
		{EntryType: "expense", Category: "utilities", Amount: 0, EntryDate: "2026-10-01", Description: "Zero"},
		{EntryType: "expense", Category: "utilities", Amount: 10, EntryDate: "2026-10-01", Description: ""},
	} {
		if _, err := s.SaveEntry(context.Background(), actor, in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("entry %d accepted: %v", i, err)
		}
	}
	for _, q := range []Query{{Status: "late"}, {Method: "cheque"}, {From: "2026-10-05", To: "2026-10-01"}, {EntryType: "income", Category: "salary"}, {PageSize: 101}, {Period: "2026-13"}} {
		if s.validQuery(&q) {
			t.Fatalf("query accepted %+v", q)
		}
	}
	from, to, ok := s.dateRange("", "")
	if !ok || from != "2026-10-01" || to != "2026-10-18" {
		t.Fatalf("default range %s %s %v", from, to, ok)
	}
	if _, _, ok := s.dateRange("2020-01-01", "2026-10-18"); ok {
		t.Fatal("unbounded report range accepted")
	}
}
