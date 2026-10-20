package fees

import "errors"

var (
	ErrInvalid     = errors.New("invalid fee input")
	ErrNotFound    = errors.New("fee record not found")
	ErrConflict    = errors.New("fee state conflict")
	ErrDuplicate   = errors.New("duplicate payment reference")
	ErrOverpayment = errors.New("payment exceeds outstanding dues")
	ErrForbidden   = errors.New("fee access denied")
	ErrDeclined    = errors.New("mobile payment declined")
	ErrExpired     = errors.New("mobile payment expired")
	// ErrOnlineDisabled: online payment is not available in this school.
	ErrOnlineDisabled = errors.New("online payment is not enabled")
	// ErrSimulationNotAllowed: the deployment does not allow simulation.
	ErrSimulationNotAllowed = errors.New("payment simulation is not allowed in this deployment")
)

type Category struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	NameEN    string `json:"name_en"`
	NameBN    string `json:"name_bn"`
	Frequency string `json:"frequency"`
	IsActive  bool   `json:"is_active"`
}
type CategoryInput struct {
	Code      string `json:"code"`
	NameEN    string `json:"name_en"`
	NameBN    string `json:"name_bn"`
	Frequency string `json:"frequency"`
	IsActive  *bool  `json:"is_active"`
}
type Structure struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	ClassID        string  `json:"class_id"`
	CategoryID     string  `json:"category_id"`
	Amount         float64 `json:"amount"`
	DueDay         int     `json:"due_day"`
	IsActive       bool    `json:"is_active"`
	SessionName    string  `json:"session_name"`
	ClassName      string  `json:"class_name"`
	CategoryCode   string  `json:"category_code"`
	CategoryNameEN string  `json:"category_name_en"`
	CategoryNameBN string  `json:"category_name_bn"`
	Frequency      string  `json:"frequency"`
}
type StructureInput struct {
	SessionID  string  `json:"session_id"`
	ClassID    string  `json:"class_id"`
	CategoryID string  `json:"category_id"`
	Amount     float64 `json:"amount"`
	DueDay     int     `json:"due_day"`
	IsActive   *bool   `json:"is_active"`
}
type Concession struct {
	ID             string  `json:"id"`
	StudentID      string  `json:"student_id"`
	CategoryID     string  `json:"category_id"`
	Percent        float64 `json:"percent"`
	Reason         string  `json:"reason"`
	StudentName    string  `json:"student_name"`
	AdmissionNo    string  `json:"admission_no"`
	CategoryNameEN string  `json:"category_name_en"`
	CategoryNameBN string  `json:"category_name_bn"`
}
type ConcessionInput struct {
	StudentID  string  `json:"student_id"`
	CategoryID string  `json:"category_id"`
	Percent    float64 `json:"percent"`
	Reason     string  `json:"reason"`
}
type Due struct {
	ID             string  `json:"id"`
	StudentID      string  `json:"student_id"`
	StructureID    string  `json:"structure_id"`
	SessionID      string  `json:"session_id"`
	ClassID        string  `json:"class_id"`
	Period         *string `json:"period"`
	DueDate        string  `json:"due_date"`
	Amount         float64 `json:"amount"`
	DiscountAmount float64 `json:"discount_amount"`
	PaidAmount     float64 `json:"paid_amount"`
	Balance        float64 `json:"balance"`
	Status         string  `json:"status"`
	Overdue        bool    `json:"overdue"`
	CategoryID     string  `json:"category_id"`
	CategoryCode   string  `json:"category_code"`
	CategoryNameEN string  `json:"category_name_en"`
	CategoryNameBN string  `json:"category_name_bn"`
	StudentName    string  `json:"student_name"`
	StudentNameBN  string  `json:"student_name_bn"`
	AdmissionNo    string  `json:"admission_no"`
	SessionName    string  `json:"session_name"`
	ClassName      string  `json:"class_name"`
}
type DueInput struct {
	StudentID   string `json:"student_id"`
	StructureID string `json:"structure_id"`
	Period      string `json:"period"`
}
type GenerateInput struct {
	SessionID      string `json:"session_id"`
	ClassID        string `json:"class_id"`
	Period         string `json:"period"`
	IncludeOneTime bool   `json:"include_one_time"`
}
type GenerateResult struct {
	Created int64 `json:"created"`
	Skipped int64 `json:"skipped"`
}
type AdjustmentInput struct {
	Kind   string  `json:"kind"`
	Amount float64 `json:"amount"`
	Reason string  `json:"reason"`
}
type Adjustment struct {
	ID           string  `json:"id"`
	StudentFeeID string  `json:"student_fee_id"`
	Kind         string  `json:"kind"`
	Amount       float64 `json:"amount"`
	Reason       string  `json:"reason"`
	CreatedAt    string  `json:"created_at"`
}
type PaymentInput struct {
	StudentID      string   `json:"student_id"`
	Amount         float64  `json:"amount"`
	Method         string   `json:"method"`
	Reference      string   `json:"reference"`
	PaidOn         string   `json:"paid_on"`
	Note           string   `json:"note"`
	IdempotencyKey string   `json:"idempotency_key"`
	FeeIDs         []string `json:"fee_ids"`
}

// PaymentRecord is a validated payment in integer poisha, ready to persist.
type PaymentRecord struct {
	StudentID      string
	AmountCents    int64
	Method         string
	Source         string
	Reference      string
	PaidOn         string
	Note           string
	IdempotencyKey string
	FeeIDs         []string
}
type Payment struct {
	ID        string  `json:"id"`
	ReceiptNo string  `json:"receipt_no"`
	StudentID string  `json:"student_id"`
	Amount    float64 `json:"amount"`
	Method    string  `json:"method"`
	Source    string  `json:"source"`
	// Simulated payments moved no money; Voided ones no longer reduce dues.
	Simulated      bool    `json:"simulated"`
	VoidedAt       *string `json:"voided_at"`
	VoidReason     string  `json:"void_reason"`
	Reference      string  `json:"reference"`
	PaidOn         string  `json:"paid_on"`
	Note           string  `json:"note"`
	ReceivedByName string  `json:"received_by_name"`
	CreatedAt      string  `json:"created_at"`
	StudentName    string  `json:"student_name"`
	StudentNameBN  string  `json:"student_name_bn"`
	AdmissionNo    string  `json:"admission_no"`
	ClassName      string  `json:"class_name"`
	SectionName    string  `json:"section_name"`
}
type Allocation struct {
	StudentFeeID   string  `json:"student_fee_id"`
	Amount         float64 `json:"amount"`
	CategoryNameEN string  `json:"category_name_en"`
	CategoryNameBN string  `json:"category_name_bn"`
	Period         *string `json:"period"`
	DueDate        string  `json:"due_date"`
}
type Receipt struct {
	Payment      Payment      `json:"payment"`
	Allocations  []Allocation `json:"allocations"`
	BalanceAfter float64      `json:"balance_after"`
	SchoolNameEN string       `json:"school_name_en"`
	SchoolNameBN string       `json:"school_name_bn"`
	LogoURL      string       `json:"logo_url"`
	Address      string       `json:"address"`
	Phone        string       `json:"phone"`
	Replayed     bool         `json:"replayed"`
}
type StudentRef struct {
	ID          string `json:"id"`
	AdmissionNo string `json:"admission_no"`
	NameEN      string `json:"name_en"`
	NameBN      string `json:"name_bn"`
	RollNo      *int   `json:"roll_no"`
	Status      string `json:"status"`
	SessionID   string `json:"session_id"`
	ClassID     string `json:"class_id"`
	SectionID   string `json:"section_id"`
	SessionName string `json:"session_name"`
	ClassName   string `json:"class_name"`
	SectionName string `json:"section_name"`
}
type Totals struct {
	Charged  float64 `json:"charged"`
	Discount float64 `json:"discount"`
	Paid     float64 `json:"paid"`
	Balance  float64 `json:"balance"`
	Overdue  float64 `json:"overdue"`
	// SimulatedUnreviewed is the part of Paid that came from simulated
	// payments recorded before the payment-safety upgrade and not yet voided:
	// no money was received for it.
	SimulatedUnreviewed float64 `json:"simulated_unreviewed"`
}
type Statement struct {
	Student  StudentRef `json:"student"`
	Dues     []Due      `json:"dues"`
	Payments []Payment  `json:"payments"`
	Totals   Totals     `json:"totals"`
}
type Intent struct {
	ID          string  `json:"id"`
	Provider    string  `json:"provider"`
	ProviderRef string  `json:"provider_ref"`
	StudentID   string  `json:"student_id"`
	PayerUserID string  `json:"-"`
	Amount      float64 `json:"amount"`
	Status      string  `json:"status"`
	ExpiresAt   string  `json:"expires_at"`
	Expired     bool    `json:"expired"`
	PaymentID   *string `json:"payment_id"`
}
type IntentInput struct {
	StudentID string  `json:"student_id"`
	Provider  string  `json:"provider"`
	Amount    float64 `json:"amount"`
}
type ConfirmInput struct {
	OTP string `json:"otp"`
}
type Entry struct {
	ID        string  `json:"id"`
	EntryType string  `json:"entry_type"`
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
	// Simulated marks income posted by a simulated payment: not real money.
	Simulated     bool    `json:"simulated"`
	EntryDate     string  `json:"entry_date"`
	Description   string  `json:"description"`
	Reference     string  `json:"reference"`
	PaymentID     *string `json:"payment_id"`
	CreatedByName string  `json:"created_by_name"`
}
type EntryInput struct {
	EntryType   string  `json:"entry_type"`
	Category    string  `json:"category"`
	Amount      float64 `json:"amount"`
	EntryDate   string  `json:"entry_date"`
	Description string  `json:"description"`
	Reference   string  `json:"reference"`
}
type Bucket struct {
	Key    string  `json:"key"`
	NameEN string  `json:"name_en"`
	NameBN string  `json:"name_bn"`
	Total  float64 `json:"total"`
	Count  int64   `json:"count"`
}
type Summary struct {
	From              string   `json:"from"`
	To                string   `json:"to"`
	Income            float64  `json:"income"`
	Expense           float64  `json:"expense"`
	Net               float64  `json:"net"`
	IncomeByCategory  []Bucket `json:"income_by_category"`
	ExpenseByCategory []Bucket `json:"expense_by_category"`
	Outstanding       float64  `json:"outstanding"`
	Arrears           float64  `json:"arrears"`
}
type CollectionReport struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Total float64 `json:"total"`
	Count int64   `json:"count"`
	// Simulated payments in the period, excluded from every figure above
	// and below.
	SimulatedTotal float64  `json:"simulated_total"`
	SimulatedCount int64    `json:"simulated_count"`
	ByMethod       []Bucket `json:"by_method"`
	ByClass        []Bucket `json:"by_class"`
	ByCategory     []Bucket `json:"by_category"`
	ByDay          []Bucket `json:"by_day"`
}
type DueReportRow struct {
	StudentID   string  `json:"student_id"`
	AdmissionNo string  `json:"admission_no"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	ClassName   string  `json:"class_name"`
	SectionName string  `json:"section_name"`
	Charged     float64 `json:"charged"`
	Discount    float64 `json:"discount"`
	Paid        float64 `json:"paid"`
	Balance     float64 `json:"balance"`
	Overdue     float64 `json:"overdue"`
	OldestDue   *string `json:"oldest_due"`
}
type DueReport struct {
	Items        []DueReportRow `json:"items"`
	Total        int64          `json:"total"`
	Page         int            `json:"page"`
	PageSize     int            `json:"page_size"`
	TotalBalance float64        `json:"total_balance"`
	TotalOverdue float64        `json:"total_overdue"`
}
type Ref struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}
type SectionRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	ClassID   string `json:"class_id"`
}
type Options struct {
	Sessions   []Ref        `json:"sessions"`
	Classes    []Ref        `json:"classes"`
	Sections   []SectionRef `json:"sections"`
	Categories []Category   `json:"categories"`
}
type Query struct {
	SessionID   string
	ClassID     string
	SectionID   string
	StudentID   string
	CategoryID  string
	Status      string
	Period      string
	Method      string
	EntryType   string
	Category    string
	From        string
	To          string
	Search      string
	OverdueOnly bool
	// Simulated filters payments: "true", "false" or "unreviewed".
	Simulated string
	Today     string
	Page      int
	PageSize  int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// OnlineStatus says whether online payment can be used and how.
type OnlineStatus struct {
	// Mode is "off" or "simulation". There is no live provider.
	Mode string `json:"mode"`
	// SimulationAllowed reports the deployment setting
	// PAYMENT_SIMULATION_ALLOWED; only administrators see it as true.
	SimulationAllowed bool   `json:"simulation_allowed"`
	SimulationEnabled bool   `json:"simulation_enabled"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type OnlineSettingsInput struct {
	SimulationEnabled bool `json:"simulation_enabled"`
}

// SimulationResult is what a confirmed simulation returns. It is not a
// receipt: no payment was recorded and no balance changed.
type SimulationResult struct {
	Simulated bool    `json:"simulated"`
	Intent    Intent  `json:"intent"`
	Balance   float64 `json:"balance"`
	Message   string  `json:"message"`
	Replayed  bool    `json:"replayed"`
}

type VoidInput struct {
	Reason string `json:"reason"`
}
