CREATE TABLE academic_sections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    name text NOT NULL,
    code text NOT NULL,
    display_order integer NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_sections_name_ck CHECK (name <> '' AND name = btrim(name) AND octet_length(name) <= 120),
    CONSTRAINT academic_sections_code_ck CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    CONSTRAINT academic_sections_order_ck CHECK (display_order BETWEEN 0 AND 100000),
    CONSTRAINT academic_sections_context_key UNIQUE (id, class_id, session_id),
    CONSTRAINT academic_sections_code_key UNIQUE (session_id, class_id, code)
);
CREATE UNIQUE INDEX academic_sections_name_key ON academic_sections (session_id, class_id, lower(name));
CREATE INDEX academic_sections_list_idx ON academic_sections (session_id, class_id, is_active, display_order, id);
CREATE INDEX academic_sections_class_idx ON academic_sections (class_id, session_id, id);
