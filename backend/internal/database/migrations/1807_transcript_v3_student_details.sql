-- Transcript v3: what the academic transcript prints about the school and
-- the student beyond the published result.
--
-- * The transcript's decorative certificate border and the faint logo
--   watermark can be turned off per school; both start on.
-- * Shifts (প্রভাতী / দিবা …) are a school-editable list; a student may
--   belong to one. Schools without shifts never see the field.
-- * The version (medium of instruction: Bangla or English) is a student
--   field; '' means not recorded, and the transcript leaves the row out.

ALTER TABLE school_settings
    ADD COLUMN transcript_border boolean NOT NULL DEFAULT true,
    ADD COLUMN transcript_watermark boolean NOT NULL DEFAULT true;

CREATE TABLE school_shifts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en text NOT NULL,
    name_bn text NOT NULL DEFAULT '',
    display_order integer NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT school_shifts_name_en_ck CHECK (name_en <> '' AND name_en = btrim(name_en) AND octet_length(name_en) <= 80),
    CONSTRAINT school_shifts_name_bn_ck CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 80),
    CONSTRAINT school_shifts_order_ck CHECK (display_order BETWEEN 0 AND 100000)
);
CREATE UNIQUE INDEX school_shifts_name_key ON school_shifts (lower(name_en));
CREATE INDEX school_shifts_active_idx ON school_shifts (is_active, display_order, id);

ALTER TABLE students
    ADD COLUMN shift_id uuid REFERENCES school_shifts(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    ADD COLUMN medium text NOT NULL DEFAULT '' CHECK (medium IN ('', 'bangla', 'english'));
CREATE INDEX students_shift_idx ON students (shift_id) WHERE shift_id IS NOT NULL;
