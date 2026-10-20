package fees

import (
	"context"
	"errors"
	"strings"
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

func TestSimulationGatewayIssuesSimulatedReferences(t *testing.T) {
	g := SimulationGateway{}
	ref, err := g.Initiate(context.Background(), "bkash", 100)
	if err != nil || len(ref) != 17 || ref[:5] != "SIMBK" {
		t.Fatalf("reference %q %v", ref, err)
	}
	other, _ := g.Initiate(context.Background(), "nagad", 100)
	if other[:5] != "SIMNG" || other == ref {
		t.Fatalf("nagad reference %q", other)
	}
	if _, err := g.Initiate(context.Background(), "rocket", 100); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown provider accepted")
	}
	if err := g.Verify(context.Background(), "bkash", ref, "000000"); !errors.Is(err, ErrDeclined) {
		t.Fatalf("wrong code: %v", err)
	}
	if err := g.Verify(context.Background(), "bkash", ref, SimulationCode); err != nil {
		t.Fatal(err)
	}
	// A reference that looks like a real provider transaction is never accepted.
	if err := g.Verify(context.Background(), "bkash", "BKABCDEFGH1234", SimulationCode); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-simulated reference: %v", err)
	}
}

// modeRepo stores the simulation switch. Its embedded Repository is nil, so
// any attempt to record a payment panics and fails the test.
type modeRepo struct {
	Repository
	enabled   bool
	saved     []bool
	simulated int
}

func (r *modeRepo) PaymentSettings(context.Context) (bool, string, error) { return r.enabled, "", nil }
func (r *modeRepo) SavePaymentSettings(_ context.Context, _ string, v bool) error {
	r.saved = append(r.saved, v)
	r.enabled = v
	return nil
}
func (r *modeRepo) ResolveStudent(context.Context, string, string, string) (string, error) {
	return student, nil
}
func (r *modeRepo) CreateIntent(_ context.Context, _, studentID, provider, ref string, cents int64, _ time.Duration) (Intent, error) {
	return Intent{ID: key, Provider: provider, ProviderRef: ref, StudentID: studentID, Amount: float64(cents) / 100, Status: "pending"}, nil
}
func (r *modeRepo) Intent(context.Context, string, string) (Intent, error) {
	return Intent{ID: key, Provider: "bkash", ProviderRef: "SIMBKABCDEFGHJKLMN", StudentID: student, Amount: 5, Status: "pending"}, nil
}
func (r *modeRepo) FailIntent(context.Context, string) error { return nil }
func (r *modeRepo) SimulateIntent(context.Context, string, string) (Intent, bool, error) {
	r.simulated++
	return Intent{ID: key, StudentID: student, Status: "simulated"}, false, nil
}
func (r *modeRepo) Statement(context.Context, string, string) (Statement, error) {
	return Statement{Totals: Totals{Balance: 1250}}, nil
}

func TestOnlinePaymentIsOffUnlessDeploymentAndAdministratorEnableIt(t *testing.T) {
	ctx := context.Background()
	start := IntentInput{Provider: "bkash", Amount: 5}

	// Production default: the deployment does not allow simulation.
	repo := &modeRepo{enabled: true} // even a switched-on setting has no effect
	s := NewService(repo, nil)
	if status, _ := s.OnlineStatus(ctx, true); status.Mode != "off" || status.SimulationAllowed {
		t.Fatalf("default status %+v", status)
	}
	if _, err := s.StartOnlinePayment(ctx, "guardian", actor, start); !errors.Is(err, ErrOnlineDisabled) {
		t.Fatalf("start: %v", err)
	}
	if _, err := s.ConfirmOnlinePayment(ctx, actor, key, ConfirmInput{OTP: SimulationCode}); !errors.Is(err, ErrOnlineDisabled) {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := s.SaveOnlineSettings(ctx, actor, OnlineSettingsInput{SimulationEnabled: true}); !errors.Is(err, ErrSimulationNotAllowed) {
		t.Fatalf("enable: %v", err)
	}
	if _, err := s.SaveOnlineSettings(ctx, actor, OnlineSettingsInput{SimulationEnabled: false}); err != nil {
		t.Fatalf("disabling must always work: %v", err)
	}

	// A test installation that allows simulation, before the administrator enables it.
	repo = &modeRepo{}
	s = NewService(repo, nil, ServiceOptions{SimulationAllowed: true})
	if _, err := s.StartOnlinePayment(ctx, "guardian", actor, start); !errors.Is(err, ErrOnlineDisabled) {
		t.Fatalf("start before enabling: %v", err)
	}
	if status, _ := s.SaveOnlineSettings(ctx, actor, OnlineSettingsInput{SimulationEnabled: true}); status.Mode != "simulation" {
		t.Fatalf("enabled status %+v", status)
	}
	if status, _ := s.OnlineStatus(ctx, false); status.SimulationAllowed || status.SimulationEnabled {
		t.Fatalf("non-administrators must not see the deployment setting: %+v", status)
	}
	intent, err := s.StartOnlinePayment(ctx, "guardian", actor, start)
	if err != nil || intent.ProviderRef[:3] != "SIM" {
		t.Fatalf("intent %+v %v", intent, err)
	}
	result, err := s.ConfirmOnlinePayment(ctx, actor, key, ConfirmInput{OTP: SimulationCode})
	if err != nil || !result.Simulated || result.Intent.Status != "simulated" || result.Balance != 1250 || repo.simulated != 1 {
		t.Fatalf("simulation %+v %v", result, err)
	}
	if !strings.Contains(result.Message, "NO MONEY TRANSFERRED") {
		t.Fatalf("message %q", result.Message)
	}
}

func TestVoidRequiresAReason(t *testing.T) {
	s := NewService(&modeRepo{}, nil)
	if _, err := s.VoidSimulatedPayment(context.Background(), actor, key, VoidInput{Reason: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("void without reason: %v", err)
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
