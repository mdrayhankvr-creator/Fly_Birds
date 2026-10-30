package fees

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	uuid         = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	categoryCode = regexp.MustCompile(`^[A-Z0-9_-]{2,20}$`)
	periodFormat = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
)

// Dhaka is Bangladesh Standard Time; the country observes no daylight saving.
var Dhaka = time.FixedZone("BST", 6*60*60)

var ledgerCategories = map[string][]string{
	"income":  {"fees", "donation", "grant", "other_income"},
	"expense": {"salary", "utilities", "supplies", "maintenance", "transport", "events", "other_expense"},
}

const intentLifetime = 15 * time.Minute

type Repository interface {
	Options(context.Context) (Options, error)
	SearchStudents(context.Context, Query) (Page[StudentRef], error)
	ListCategories(context.Context, Query) (Page[Category], error)
	SaveCategory(context.Context, string, string, CategoryInput) (Category, error)
	ListStructures(context.Context, Query) (Page[Structure], error)
	SaveStructure(context.Context, string, string, StructureInput, int64) (Structure, error)
	ListConcessions(context.Context, Query) (Page[Concession], error)
	SaveConcession(context.Context, string, ConcessionInput) (Concession, error)
	DeleteConcession(context.Context, string, string) error
	ReviewConcessions(context.Context, string, string) ([]ConcessionReview, error)
	ListDues(context.Context, Query) (Page[Due], error)
	AssignDue(context.Context, string, DueInput) (Due, error)
	GenerateDues(context.Context, string, GenerateInput) (GenerateResult, error)
	DeleteDue(context.Context, string, string) error
	Adjust(context.Context, string, string, string, int64, string) (Due, error)
	Adjustments(context.Context, string) ([]Adjustment, error)
	RecordPayment(context.Context, string, PaymentRecord) (string, bool, error)
	ListPayments(context.Context, Query) (Page[Payment], error)
	Receipt(context.Context, string, string) (Receipt, error)
	ResolveStudent(context.Context, string, string, string) (string, error)
	Statement(context.Context, string, string) (Statement, error)
	CreateIntent(context.Context, string, string, string, string, int64, time.Duration) (Intent, error)
	Intent(context.Context, string, string) (Intent, error)
	FailIntent(context.Context, string) error
	SimulateIntent(context.Context, string, string) (Intent, bool, error)
	PaymentSettings(context.Context) (bool, string, error)
	SavePaymentSettings(context.Context, string, bool) error
	VoidSimulatedPayment(context.Context, string, string, string) (Payment, error)
	ListEntries(context.Context, Query) (Page[Entry], error)
	SaveEntry(context.Context, string, EntryInput, int64) (Entry, error)
	DeleteEntry(context.Context, string, string) error
	Summary(context.Context, string, string, string) (Summary, error)
	Collections(context.Context, Query) (CollectionReport, error)
	DueReport(context.Context, Query) (DueReport, error)
}

type Service struct {
	repo    Repository
	gateway Gateway
	now     func() time.Time
	// simulationAllowed is the deployment's PAYMENT_SIMULATION_ALLOWED. Without
	// it no online payment is possible, whatever the administrator chooses.
	simulationAllowed bool
}

// ServiceOptions are deployment settings for the fee service.
type ServiceOptions struct {
	SimulationAllowed bool
}

// NewService builds the fee service. Online payment is off unless the
// deployment allows simulation and an administrator enables it; a nil
// gateway uses the simulator, the only gateway that exists.
func NewService(repo Repository, gateway Gateway, opts ...ServiceOptions) *Service {
	if gateway == nil {
		gateway = SimulationGateway{}
	}
	s := &Service{repo: repo, gateway: gateway, now: time.Now}
	for _, o := range opts {
		s.simulationAllowed = o.SimulationAllowed
	}
	return s
}

func (s *Service) today() string { return s.now().In(Dhaka).Format(time.DateOnly) }

func text(v string, max int, required bool) (string, bool) {
	v = strings.TrimSpace(v)
	return v, utf8.ValidString(v) && len(v) <= max && (!required || v != "")
}
func optionalID(v string) bool { return v == "" || uuid.MatchString(v) }
func validDate(v string) bool {
	d, err := time.Parse(time.DateOnly, v)
	return err == nil && d.Year() >= 2000 && d.Year() <= 2200
}

// ValidQuery checks the shared list filters and fills the default page.
func (s *Service) validQuery(q *Query) bool {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	q.Search = strings.TrimSpace(q.Search)
	q.Today = s.today()
	ok := q.Page >= 1 && q.Page <= 100000 && q.PageSize >= 1 && q.PageSize <= 100 && len(q.Search) <= 100 && utf8.ValidString(q.Search) &&
		optionalID(q.SessionID) && optionalID(q.ClassID) && optionalID(q.SectionID) && optionalID(q.StudentID) && optionalID(q.CategoryID) &&
		(q.Period == "" || periodFormat.MatchString(q.Period)) && (q.From == "" || validDate(q.From)) && (q.To == "" || validDate(q.To))
	switch q.Status {
	case "", "unpaid", "partial", "paid", "waived", "overdue", "outstanding":
	default:
		return false
	}
	switch q.Simulated {
	case "", "true", "false", "unreviewed":
	default:
		return false
	}
	switch q.Method {
	case "", "cash", "bank", "bkash", "nagad":
	default:
		return false
	}
	switch q.EntryType {
	case "", "income", "expense":
	default:
		return false
	}
	if q.Category != "" && !validLedgerCategory(q.EntryType, q.Category) {
		return false
	}
	return ok && (q.From == "" || q.To == "" || q.From <= q.To)
}
func validLedgerCategory(kind, category string) bool {
	for k, list := range ledgerCategories {
		if kind != "" && k != kind {
			continue
		}
		for _, v := range list {
			if v == category {
				return true
			}
		}
	}
	return false
}

// dateRange defaults reports to the current calendar month and limits them to
// about two years so a single request stays bounded.
func (s *Service) dateRange(from, to string) (string, string, bool) {
	now := s.now().In(Dhaka)
	if from == "" {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, Dhaka).Format(time.DateOnly)
	}
	if to == "" {
		to = now.Format(time.DateOnly)
	}
	if !validDate(from) || !validDate(to) || from > to {
		return "", "", false
	}
	f, _ := time.Parse(time.DateOnly, from)
	t, _ := time.Parse(time.DateOnly, to)
	return from, to, t.Sub(f) <= 2*366*24*time.Hour
}

func (s *Service) Options(ctx context.Context) (Options, error) { return s.repo.Options(ctx) }
func (s *Service) SearchStudents(ctx context.Context, q Query) (Page[StudentRef], error) {
	if !s.validQuery(&q) {
		return Page[StudentRef]{}, ErrInvalid
	}
	return s.repo.SearchStudents(ctx, q)
}
func (s *Service) ListCategories(ctx context.Context, q Query) (Page[Category], error) {
	if !s.validQuery(&q) {
		return Page[Category]{}, ErrInvalid
	}
	return s.repo.ListCategories(ctx, q)
}
func (s *Service) SaveCategory(ctx context.Context, actor, id string, in CategoryInput) (Category, error) {
	var ok1, ok2 bool
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.NameEN, ok1 = text(in.NameEN, 120, true)
	in.NameBN, ok2 = text(in.NameBN, 240, false)
	if !uuid.MatchString(actor) || !optionalID(id) || !ok1 || !ok2 || !categoryCode.MatchString(in.Code) || (in.Frequency != "monthly" && in.Frequency != "one_time") {
		return Category{}, ErrInvalid
	}
	return s.repo.SaveCategory(ctx, actor, id, in)
}
func (s *Service) ListStructures(ctx context.Context, q Query) (Page[Structure], error) {
	if !s.validQuery(&q) {
		return Page[Structure]{}, ErrInvalid
	}
	return s.repo.ListStructures(ctx, q)
}
func (s *Service) SaveStructure(ctx context.Context, actor, id string, in StructureInput) (Structure, error) {
	cents, ok := ToCents(in.Amount)
	if in.DueDay == 0 {
		in.DueDay = 10
	}
	if !uuid.MatchString(actor) || !optionalID(id) || !uuid.MatchString(in.SessionID) || !uuid.MatchString(in.ClassID) || !uuid.MatchString(in.CategoryID) || !ok || cents <= 0 || cents > 1_000_000_000 || in.DueDay < 1 || in.DueDay > 28 {
		return Structure{}, ErrInvalid
	}
	return s.repo.SaveStructure(ctx, actor, id, in, cents)
}
func (s *Service) ListConcessions(ctx context.Context, q Query) (Page[Concession], error) {
	if !s.validQuery(&q) {
		return Page[Concession]{}, ErrInvalid
	}
	return s.repo.ListConcessions(ctx, q)
}
func (s *Service) SaveConcession(ctx context.Context, actor string, in ConcessionInput) (Concession, error) {
	var ok bool
	in.Reason, ok = text(in.Reason, 240, true)
	basis, valid := ToCents(in.Percent)
	if !uuid.MatchString(actor) || !uuid.MatchString(in.StudentID) || !uuid.MatchString(in.CategoryID) || !ok || !valid || basis <= 0 || basis > 10000 || (in.SessionID != "" && !uuid.MatchString(in.SessionID)) {
		return Concession{}, ErrInvalid
	}
	return s.repo.SaveConcession(ctx, actor, in)
}
func (s *Service) ReviewConcessions(ctx context.Context, from, to string) ([]ConcessionReview, error) {
	if !uuid.MatchString(from) || !uuid.MatchString(to) || from == to {
		return nil, ErrInvalid
	}
	return s.repo.ReviewConcessions(ctx, from, to)
}
func (s *Service) DeleteConcession(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteConcession(ctx, actor, id)
}
func (s *Service) ListDues(ctx context.Context, q Query) (Page[Due], error) {
	if !s.validQuery(&q) {
		return Page[Due]{}, ErrInvalid
	}
	return s.repo.ListDues(ctx, q)
}
func (s *Service) AssignDue(ctx context.Context, actor string, in DueInput) (Due, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(in.StudentID) || !uuid.MatchString(in.StructureID) || (in.Period != "" && !periodFormat.MatchString(in.Period)) {
		return Due{}, ErrInvalid
	}
	return s.repo.AssignDue(ctx, actor, in)
}
func (s *Service) GenerateDues(ctx context.Context, actor string, in GenerateInput) (GenerateResult, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(in.SessionID) || !optionalID(in.ClassID) || !periodFormat.MatchString(in.Period) {
		return GenerateResult{}, ErrInvalid
	}
	return s.repo.GenerateDues(ctx, actor, in)
}
func (s *Service) DeleteDue(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteDue(ctx, actor, id)
}

// Adjust applies a partial discount or a full waiver of the remaining balance.
// Waivers write off money and are limited to administrators.
func (s *Service) Adjust(ctx context.Context, actor, role, dueID string, in AdjustmentInput) (Due, error) {
	reason, ok := text(in.Reason, 240, true)
	cents, valid := ToCents(in.Amount)
	if !uuid.MatchString(actor) || !uuid.MatchString(dueID) || !ok || !valid {
		return Due{}, ErrInvalid
	}
	switch in.Kind {
	case "discount":
		if cents <= 0 {
			return Due{}, ErrInvalid
		}
	case "waiver":
		if role != "admin" {
			return Due{}, ErrForbidden
		}
		cents = 0 // the repository waives exactly the locked remaining balance
	default:
		return Due{}, ErrInvalid
	}
	return s.repo.Adjust(ctx, actor, in.Kind, dueID, cents, reason)
}
func (s *Service) Adjustments(ctx context.Context, dueID string) ([]Adjustment, error) {
	if !uuid.MatchString(dueID) {
		return nil, ErrInvalid
	}
	return s.repo.Adjustments(ctx, dueID)
}

// RecordPayment stores a counter payment. Replaying the same idempotency key
// returns the original receipt instead of charging twice.
func (s *Service) RecordPayment(ctx context.Context, actor string, in PaymentInput) (Receipt, error) {
	cents, ok := ToCents(in.Amount)
	reference, refOK := text(in.Reference, 80, in.Method != "cash")
	note, noteOK := text(in.Note, 240, false)
	if in.PaidOn == "" {
		in.PaidOn = s.today()
	}
	if !uuid.MatchString(actor) || !uuid.MatchString(in.StudentID) || !ok || cents <= 0 || !refOK || !noteOK || !uuid.MatchString(in.IdempotencyKey) || !validDate(in.PaidOn) || in.PaidOn > s.today() || len(in.FeeIDs) > 200 {
		return Receipt{}, ErrInvalid
	}
	switch in.Method {
	case "cash", "bank", "bkash", "nagad":
	default:
		return Receipt{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range in.FeeIDs {
		if !uuid.MatchString(id) || seen[strings.ToLower(id)] {
			return Receipt{}, ErrInvalid
		}
		seen[strings.ToLower(id)] = true
	}
	id, replay, err := s.repo.RecordPayment(ctx, actor, PaymentRecord{StudentID: in.StudentID, AmountCents: cents, Method: in.Method, Source: "counter", Reference: reference, PaidOn: in.PaidOn, Note: note, IdempotencyKey: in.IdempotencyKey, FeeIDs: in.FeeIDs})
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := s.repo.Receipt(ctx, id, "")
	receipt.Replayed = replay
	return receipt, err
}
func (s *Service) ListPayments(ctx context.Context, q Query) (Page[Payment], error) {
	if !s.validQuery(&q) {
		return Page[Payment]{}, ErrInvalid
	}
	return s.repo.ListPayments(ctx, q)
}

// Receipt reads one receipt. Staff may read any; students and guardians only
// receipts that belong to their own or a linked child's account.
func (s *Service) Receipt(ctx context.Context, scope, actor, studentID, paymentID string) (Receipt, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(paymentID) || !optionalID(studentID) {
		return Receipt{}, ErrInvalid
	}
	owner := ""
	if scope != "staff" {
		var err error
		if owner, err = s.repo.ResolveStudent(ctx, scope, actor, studentID); err != nil {
			return Receipt{}, err
		}
	}
	return s.repo.Receipt(ctx, paymentID, owner)
}
func (s *Service) Statement(ctx context.Context, scope, actor, studentID string) (Statement, error) {
	if !uuid.MatchString(actor) || !optionalID(studentID) {
		return Statement{}, ErrInvalid
	}
	id := studentID
	if scope != "staff" {
		var err error
		if id, err = s.repo.ResolveStudent(ctx, scope, actor, studentID); err != nil {
			return Statement{}, err
		}
	} else if id == "" {
		return Statement{}, ErrInvalid
	}
	return s.repo.Statement(ctx, id, s.today())
}

// OnlineStatus reports whether online payment can be used. The mode is
// "simulation" only when the deployment allows it and an administrator has
// switched it on; otherwise "off". The deployment setting is shown to
// administrators only.
func (s *Service) OnlineStatus(ctx context.Context, administrator bool) (OnlineStatus, error) {
	enabled, updated, err := s.repo.PaymentSettings(ctx)
	if err != nil {
		return OnlineStatus{}, err
	}
	out := OnlineStatus{Mode: "off"}
	if s.simulationAllowed && enabled {
		out.Mode = "simulation"
	}
	if administrator {
		out.SimulationAllowed, out.SimulationEnabled, out.UpdatedAt = s.simulationAllowed, enabled, updated
	}
	return out, nil
}

func (s *Service) simulating(ctx context.Context) error {
	status, err := s.OnlineStatus(ctx, false)
	if err != nil {
		return err
	}
	if status.Mode != "simulation" {
		return ErrOnlineDisabled
	}
	return nil
}

// SaveOnlineSettings is the administrator's switch. Turning simulation on is
// refused unless the deployment allows it; turning it off always works.
func (s *Service) SaveOnlineSettings(ctx context.Context, actor string, in OnlineSettingsInput) (OnlineStatus, error) {
	if !uuid.MatchString(actor) {
		return OnlineStatus{}, ErrInvalid
	}
	if in.SimulationEnabled && !s.simulationAllowed {
		return OnlineStatus{}, ErrSimulationNotAllowed
	}
	if err := s.repo.SavePaymentSettings(ctx, actor, in.SimulationEnabled); err != nil {
		return OnlineStatus{}, err
	}
	return s.OnlineStatus(ctx, true)
}

// StartOnlinePayment opens a simulated bKash or Nagad checkout for the
// payer's own or linked child's outstanding dues. It is refused unless
// simulation is enabled; there is no live provider.
func (s *Service) StartOnlinePayment(ctx context.Context, scope, actor string, in IntentInput) (Intent, error) {
	cents, ok := ToCents(in.Amount)
	if !uuid.MatchString(actor) || !optionalID(in.StudentID) || !ok || cents <= 0 || (in.Provider != "bkash" && in.Provider != "nagad") || (scope != "student" && scope != "guardian") {
		return Intent{}, ErrInvalid
	}
	if err := s.simulating(ctx); err != nil {
		return Intent{}, err
	}
	studentID, err := s.repo.ResolveStudent(ctx, scope, actor, in.StudentID)
	if err != nil {
		return Intent{}, err
	}
	reference, err := s.gateway.Initiate(ctx, in.Provider, cents)
	if err != nil {
		return Intent{}, err
	}
	return s.repo.CreateIntent(ctx, actor, studentID, in.Provider, reference, cents, intentLifetime)
}

// ConfirmOnlinePayment checks a simulated checkout's code. Success closes the
// checkout as a simulation: no payment, receipt or ledger income is recorded
// and no balance changes. It is refused whenever simulation is not enabled,
// including for checkouts started before it was switched off.
func (s *Service) ConfirmOnlinePayment(ctx context.Context, actor, intentID string, in ConfirmInput) (SimulationResult, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(intentID) || len(in.OTP) > 12 {
		return SimulationResult{}, ErrInvalid
	}
	if err := s.simulating(ctx); err != nil {
		return SimulationResult{}, err
	}
	intent, err := s.repo.Intent(ctx, actor, intentID)
	if err != nil {
		return SimulationResult{}, err
	}
	if intent.Status == "pending" && !intent.Expired {
		if err := s.gateway.Verify(ctx, intent.Provider, intent.ProviderRef, in.OTP); err != nil {
			if errors.Is(err, ErrDeclined) {
				if failErr := s.repo.FailIntent(ctx, intentID); failErr != nil {
					return SimulationResult{}, failErr
				}
			}
			return SimulationResult{}, err
		}
	}
	closed, replay, err := s.repo.SimulateIntent(ctx, actor, intentID)
	if err != nil {
		return SimulationResult{}, err
	}
	statement, err := s.repo.Statement(ctx, closed.StudentID, s.today())
	if err != nil {
		return SimulationResult{}, err
	}
	return SimulationResult{Simulated: true, Intent: closed, Balance: statement.Totals.Balance, Replayed: replay,
		Message: "SIMULATED — NO MONEY TRANSFERRED. No payment was recorded and the balance is unchanged."}, nil
}

// VoidSimulatedPayment is the administrator's explicit, audited correction
// for a simulated payment recorded before the payment-safety upgrade.
func (s *Service) VoidSimulatedPayment(ctx context.Context, actor, id string, in VoidInput) (Payment, error) {
	reason, ok := text(in.Reason, 240, true)
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || !ok {
		return Payment{}, ErrInvalid
	}
	return s.repo.VoidSimulatedPayment(ctx, actor, id, reason)
}

func (s *Service) ListEntries(ctx context.Context, q Query) (Page[Entry], error) {
	if !s.validQuery(&q) {
		return Page[Entry]{}, ErrInvalid
	}
	return s.repo.ListEntries(ctx, q)
}

// SaveEntry records a manual income or expense. Fee income is posted only by
// payments so the fee ledger always reconciles with receipts.
func (s *Service) SaveEntry(ctx context.Context, actor string, in EntryInput) (Entry, error) {
	var ok1, ok2 bool
	in.Description, ok1 = text(in.Description, 240, true)
	in.Reference, ok2 = text(in.Reference, 80, false)
	cents, ok := ToCents(in.Amount)
	if !uuid.MatchString(actor) || !ok1 || !ok2 || !ok || cents <= 0 || in.EntryType == "" || !validLedgerCategory(in.EntryType, in.Category) || in.Category == "fees" || !validDate(in.EntryDate) || in.EntryDate > s.today() {
		return Entry{}, ErrInvalid
	}
	return s.repo.SaveEntry(ctx, actor, in, cents)
}
func (s *Service) DeleteEntry(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteEntry(ctx, actor, id)
}
func (s *Service) Summary(ctx context.Context, from, to string) (Summary, error) {
	from, to, ok := s.dateRange(from, to)
	if !ok {
		return Summary{}, ErrInvalid
	}
	return s.repo.Summary(ctx, from, to, s.today())
}
func (s *Service) Collections(ctx context.Context, q Query) (CollectionReport, error) {
	var ok bool
	if q.From, q.To, ok = s.dateRange(q.From, q.To); !ok || !s.validQuery(&q) {
		return CollectionReport{}, ErrInvalid
	}
	return s.repo.Collections(ctx, q)
}
func (s *Service) DueReport(ctx context.Context, q Query) (DueReport, error) {
	if !s.validQuery(&q) {
		return DueReport{}, ErrInvalid
	}
	return s.repo.DueReport(ctx, q)
}
