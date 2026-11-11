-- Admit cards (প্রবেশপত্র), made from a term's published exam routine (E8).
--
-- * Settings: the exam rules printed on every card (English and Bangla;
--   empty uses the built-in rules), and an optional hold: when on, a
--   student with a fee due on or before the cutoff date that is still
--   unpaid gets no admit card until the office releases them. Off by
--   default.
-- * A release lets one student have their card for one term despite dues.
-- * Each printed card carries a verification code for its QR: the term,
--   the student and the routine version it was made from.

CREATE TABLE admit_card_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    rules_en text NOT NULL DEFAULT '' CHECK (char_length(rules_en) <= 2000),
    rules_bn text NOT NULL DEFAULT '' CHECK (char_length(rules_bn) <= 2000),
    withhold_for_dues boolean NOT NULL DEFAULT false,
    dues_cutoff date,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT admit_card_settings_cutoff_ck CHECK (NOT withhold_for_dues OR dues_cutoff IS NOT NULL)
);
INSERT INTO admit_card_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE admit_card_releases (
    term_id uuid NOT NULL REFERENCES exam_terms(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    note text NOT NULL DEFAULT '' CHECK (note = btrim(note) AND char_length(note) <= 240),
    released_by uuid REFERENCES users(id) ON DELETE SET NULL,
    released_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (term_id, student_id)
);

CREATE TABLE admit_card_tokens (
    token text PRIMARY KEY CHECK (token ~ '^[A-Za-z0-9_-]{22}$'),
    term_id uuid NOT NULL REFERENCES exam_terms(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    routine_version_id uuid NOT NULL REFERENCES routine_versions(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT admit_card_tokens_key UNIQUE (routine_version_id, student_id)
);

INSERT INTO permissions (id, description) VALUES
    ('routine:admit_cards', 'Print admit cards (teachers: the sections they are class teacher of) and see the withheld list'),
    ('routine:admit_settings', 'Change the admit card rules and the fee hold'),
    ('routine:admit_release', 'See students whose admit cards are held for dues, and release them')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'routine:admit_cards'), ('head_teacher', 'routine:admit_cards'), ('teacher', 'routine:admit_cards'),
    ('admin', 'routine:admit_settings'),
    ('admin', 'routine:admit_release'), ('accountant', 'routine:admit_release')
ON CONFLICT DO NOTHING;
