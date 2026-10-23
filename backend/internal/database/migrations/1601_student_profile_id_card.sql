-- Student profile details and double-sided ID cards. Every new column is
-- optional with a safe default, so existing students stay valid unchanged.
-- Parents are recorded by name only; the guardian shown on profiles and
-- cards is the existing primary student_guardians link, never a copy.
ALTER TABLE students
    ADD COLUMN student_code text NOT NULL DEFAULT '',
    ADD COLUMN gender text NOT NULL DEFAULT '',
    ADD COLUMN blood_group text NOT NULL DEFAULT '',
    ADD COLUMN father_name_en text NOT NULL DEFAULT '',
    ADD COLUMN father_name_bn text NOT NULL DEFAULT '',
    ADD COLUMN mother_name_en text NOT NULL DEFAULT '',
    ADD COLUMN mother_name_bn text NOT NULL DEFAULT '',
    ADD COLUMN present_address text NOT NULL DEFAULT '',
    ADD COLUMN permanent_address text NOT NULL DEFAULT '',
    ADD COLUMN admission_date date,
    -- Opaque random value behind the card's verification QR; reissuing a
    -- lost card replaces it, which invalidates the old card.
    ADD COLUMN card_token text,
    ADD CONSTRAINT students_student_code_ck CHECK (student_code = btrim(student_code) AND octet_length(student_code) <= 64),
    ADD CONSTRAINT students_gender_ck CHECK (gender IN ('', 'male', 'female', 'other')),
    ADD CONSTRAINT students_blood_group_ck CHECK (blood_group IN ('', 'A+', 'A-', 'B+', 'B-', 'AB+', 'AB-', 'O+', 'O-')),
    ADD CONSTRAINT students_parent_names_ck CHECK (
        father_name_en = btrim(father_name_en) AND octet_length(father_name_en) <= 160
        AND father_name_bn = btrim(father_name_bn) AND octet_length(father_name_bn) <= 160
        AND mother_name_en = btrim(mother_name_en) AND octet_length(mother_name_en) <= 160
        AND mother_name_bn = btrim(mother_name_bn) AND octet_length(mother_name_bn) <= 160),
    ADD CONSTRAINT students_addresses_ck CHECK (
        present_address = btrim(present_address) AND octet_length(present_address) <= 500
        AND permanent_address = btrim(permanent_address) AND octet_length(permanent_address) <= 500),
    ADD CONSTRAINT students_card_token_ck CHECK (card_token IS NULL OR card_token ~ '^[A-Za-z0-9_-]{22}$');
CREATE UNIQUE INDEX students_student_code_key ON students (lower(student_code)) WHERE student_code <> '';
CREATE UNIQUE INDEX students_card_token_key ON students (card_token) WHERE card_token IS NOT NULL;

CREATE TABLE id_card_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    -- NULL means each card is valid until its academic session ends.
    valid_until date,
    signatory_title_en text NOT NULL DEFAULT 'Principal',
    signatory_title_bn text NOT NULL DEFAULT 'অধ্যক্ষ',
    signature_type text CHECK (signature_type IN ('image/png', 'image/jpeg')),
    signature bytea CHECK (signature IS NULL OR octet_length(signature) BETWEEN 1 AND 524288),
    emergency_phone text NOT NULL DEFAULT '',
    -- Empty texts use the built-in bilingual defaults.
    usage_note_en text NOT NULL DEFAULT '',
    usage_note_bn text NOT NULL DEFAULT '',
    return_note_en text NOT NULL DEFAULT '',
    return_note_bn text NOT NULL DEFAULT '',
    show_qr boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT id_card_settings_signature_ck CHECK ((signature IS NULL) = (signature_type IS NULL)),
    CONSTRAINT id_card_settings_text_ck CHECK (
        octet_length(signatory_title_en) <= 80 AND octet_length(signatory_title_bn) <= 160
        AND emergency_phone = btrim(emergency_phone) AND octet_length(emergency_phone) <= 32
        AND octet_length(usage_note_en) <= 400 AND octet_length(usage_note_bn) <= 800
        AND octet_length(return_note_en) <= 400 AND octet_length(return_note_bn) <= 800)
);
INSERT INTO id_card_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
