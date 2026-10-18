-- The demo seed (cmd/seed-demo) records every row it creates here, keyed by a
-- stable reference. Re-running the seed looks the reference up instead of
-- inserting again, and it never updates or deletes a row it did not create.
CREATE TABLE IF NOT EXISTS demo_records (
    kind text NOT NULL CHECK (kind ~ '^[a-z_]{2,40}$'),
    ref text NOT NULL CHECK (ref <> '' AND octet_length(ref) <= 120),
    record_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, ref)
);
