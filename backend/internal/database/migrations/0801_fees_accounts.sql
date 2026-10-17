CREATE TABLE fee_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL CHECK (code ~ '^[A-Z0-9_-]{2,20}$'),
    name_en text NOT NULL CHECK (name_en = btrim(name_en) AND name_en <> '' AND octet_length(name_en) <= 120),
    name_bn text NOT NULL DEFAULT '' CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 240),
    frequency text NOT NULL CHECK (frequency IN ('monthly', 'one_time')),
    is_active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX fee_categories_code_key ON fee_categories(code);

CREATE TABLE fee_structures (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    category_id uuid NOT NULL REFERENCES fee_categories(id) ON DELETE RESTRICT,
    amount numeric(12,2) NOT NULL CHECK (amount > 0 AND amount <= 10000000),
    due_day smallint NOT NULL DEFAULT 10 CHECK (due_day BETWEEN 1 AND 28),
    is_active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fee_structures_key UNIQUE (session_id, class_id, category_id),
    CONSTRAINT fee_structures_context_key UNIQUE (id, session_id, class_id)
);
CREATE INDEX fee_structures_category_idx ON fee_structures(category_id);

CREATE TABLE fee_concessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES fee_categories(id) ON DELETE RESTRICT,
    percent numeric(5,2) NOT NULL CHECK (percent > 0 AND percent <= 100),
    reason text NOT NULL CHECK (reason = btrim(reason) AND reason <> '' AND octet_length(reason) <= 240),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fee_concessions_key UNIQUE (student_id, category_id)
);

-- Dues snapshot the structure amount so later structure edits never rewrite history.
CREATE TABLE student_fees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    structure_id uuid NOT NULL,
    session_id uuid NOT NULL,
    class_id uuid NOT NULL,
    period date CHECK (period IS NULL OR EXTRACT(DAY FROM period) = 1),
    due_date date NOT NULL,
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    discount_amount numeric(12,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    paid_amount numeric(12,2) NOT NULL DEFAULT 0 CHECK (paid_amount >= 0),
    balance numeric(12,2) GENERATED ALWAYS AS (amount - discount_amount - paid_amount) STORED,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT student_fees_structure_fk FOREIGN KEY (structure_id, session_id, class_id) REFERENCES fee_structures(id, session_id, class_id) ON DELETE RESTRICT,
    CONSTRAINT student_fees_no_overpayment CHECK (discount_amount + paid_amount <= amount),
    CONSTRAINT student_fees_period_key UNIQUE NULLS NOT DISTINCT (student_id, structure_id, period)
);
CREATE INDEX student_fees_student_idx ON student_fees(student_id, due_date, created_at);
CREATE INDEX student_fees_context_idx ON student_fees(session_id, class_id, due_date);
CREATE INDEX student_fees_outstanding_idx ON student_fees(due_date) WHERE balance > 0;

CREATE TABLE fee_adjustments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_fee_id uuid NOT NULL REFERENCES student_fees(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('discount', 'waiver')),
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    reason text NOT NULL CHECK (reason = btrim(reason) AND reason <> '' AND octet_length(reason) <= 240),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fee_adjustments_fee_idx ON fee_adjustments(student_fee_id);

CREATE SEQUENCE fee_receipt_seq;
CREATE TABLE fee_payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_no text NOT NULL DEFAULT ('RCT-' || lpad(nextval('fee_receipt_seq')::text, 8, '0')) UNIQUE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    method text NOT NULL CHECK (method IN ('cash', 'bank', 'bkash', 'nagad')),
    source text NOT NULL CHECK (source IN ('counter', 'online')),
    reference text NOT NULL DEFAULT '' CHECK (reference = btrim(reference) AND octet_length(reference) <= 80),
    paid_on date NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (note = btrim(note) AND octet_length(note) <= 240),
    idempotency_key uuid NOT NULL UNIQUE,
    received_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fee_payments_reference_required CHECK (method = 'cash' OR reference <> '')
);
CREATE UNIQUE INDEX fee_payments_reference_key ON fee_payments(method, upper(reference)) WHERE method <> 'cash';
CREATE INDEX fee_payments_student_idx ON fee_payments(student_id, paid_on);
CREATE INDEX fee_payments_date_idx ON fee_payments(paid_on, method);

CREATE TABLE fee_payment_allocations (
    payment_id uuid NOT NULL REFERENCES fee_payments(id) ON DELETE RESTRICT,
    student_fee_id uuid NOT NULL REFERENCES student_fees(id) ON DELETE RESTRICT,
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (payment_id, student_fee_id)
);
CREATE INDEX fee_payment_allocations_fee_idx ON fee_payment_allocations(student_fee_id);

CREATE TABLE online_payment_intents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider text NOT NULL CHECK (provider IN ('bkash', 'nagad')),
    provider_ref text NOT NULL UNIQUE CHECK (provider_ref ~ '^[A-Z0-9]{8,40}$'),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    payer_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'failed')),
    payment_id uuid UNIQUE REFERENCES fee_payments(id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT online_payment_intents_completed CHECK ((status = 'completed') = (payment_id IS NOT NULL))
);
CREATE INDEX online_payment_intents_payer_idx ON online_payment_intents(payer_user_id, created_at);

CREATE TABLE ledger_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_type text NOT NULL CHECK (entry_type IN ('income', 'expense')),
    category text NOT NULL,
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    entry_date date NOT NULL,
    description text NOT NULL CHECK (description = btrim(description) AND description <> '' AND octet_length(description) <= 240),
    reference text NOT NULL DEFAULT '' CHECK (reference = btrim(reference) AND octet_length(reference) <= 80),
    payment_id uuid UNIQUE REFERENCES fee_payments(id) ON DELETE RESTRICT,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ledger_entries_category_ck CHECK (
        (entry_type = 'income' AND category IN ('fees', 'donation', 'grant', 'other_income')) OR
        (entry_type = 'expense' AND category IN ('salary', 'utilities', 'supplies', 'maintenance', 'transport', 'events', 'other_expense'))
    ),
    CONSTRAINT ledger_entries_fee_link CHECK (payment_id IS NULL OR (entry_type = 'income' AND category = 'fees'))
);
CREATE INDEX ledger_entries_date_idx ON ledger_entries(entry_date, entry_type, category);

INSERT INTO permissions(id, description) VALUES
 ('fees:read', 'Read fee setup, dues, payments, receipts and collection reports'),
 ('fees:manage', 'Manage fee categories, structures, concessions, dues and adjustments'),
 ('fees:collect', 'Record fee payments and issue receipts'),
 ('fees:read_self', 'Read own fee statement and receipts'),
 ('fees:read_children', 'Read linked children fee statements and receipts'),
 ('fees:pay_online', 'Pay own or linked children dues through mock bKash or Nagad'),
 ('accounts:read', 'Read income, expense ledger and accounting summaries'),
 ('accounts:manage', 'Record and remove manual income and expense entries')
ON CONFLICT(id) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id) VALUES
 ('admin', 'fees:read'), ('admin', 'fees:manage'), ('admin', 'fees:collect'), ('admin', 'accounts:read'), ('admin', 'accounts:manage'),
 ('accountant', 'fees:read'), ('accountant', 'fees:manage'), ('accountant', 'fees:collect'), ('accountant', 'accounts:read'), ('accountant', 'accounts:manage'),
 ('student', 'fees:read_self'), ('student', 'fees:pay_online'),
 ('guardian', 'fees:read_children'), ('guardian', 'fees:pay_online')
ON CONFLICT DO NOTHING;
