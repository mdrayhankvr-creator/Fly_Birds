-- Uploaded files live on disk under a random server-generated key; PostgreSQL
-- keeps only metadata and ownership. Purpose limits where a file may be linked.
CREATE TABLE file_objects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key text NOT NULL UNIQUE CHECK (storage_key ~ '^[0-9a-f]{32}$'),
    purpose text NOT NULL CHECK (purpose IN ('notice', 'diary', 'homework')),
    original_name text NOT NULL CHECK (original_name <> '' AND octet_length(original_name) <= 200 AND original_name !~ '[/\\]'),
    content_type text NOT NULL CHECK (content_type IN ('application/pdf', 'image/jpeg', 'image/png')),
    size_bytes integer NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    uploaded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    linked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX file_objects_unlinked_idx ON file_objects(created_at) WHERE linked_at IS NULL;

CREATE TABLE notices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title_en text NOT NULL CHECK (title_en = btrim(title_en) AND title_en <> '' AND octet_length(title_en) <= 200),
    title_bn text NOT NULL DEFAULT '' CHECK (title_bn = btrim(title_bn) AND octet_length(title_bn) <= 600),
    body_en text NOT NULL DEFAULT '' CHECK (octet_length(body_en) <= 20000),
    body_bn text NOT NULL DEFAULT '' CHECK (octet_length(body_bn) <= 60000),
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('normal', 'important', 'urgent')),
    audience_roles text[] NOT NULL CHECK (cardinality(audience_roles) > 0 AND audience_roles <@ ARRAY['head_teacher','teacher','student','guardian','accountant']::text[]),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    publish_at timestamptz,
    expires_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notices_body_ck CHECK (body_en <> '' OR body_bn <> ''),
    CONSTRAINT notices_published_ck CHECK (status = 'draft' OR publish_at IS NOT NULL),
    CONSTRAINT notices_expiry_ck CHECK (expires_at IS NULL OR publish_at IS NULL OR expires_at > publish_at)
);
CREATE INDEX notices_feed_idx ON notices(status, publish_at DESC);

-- Optional class or section targeting. With no rows a notice reaches every
-- member of its audience roles; with rows, students and guardians must match a
-- target and teachers must teach a targeted class.
CREATE TABLE notice_targets (
    notice_id uuid NOT NULL REFERENCES notices(id) ON DELETE CASCADE,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    CONSTRAINT notice_targets_key UNIQUE NULLS NOT DISTINCT (notice_id, class_id, section_id)
);

CREATE TABLE notice_attachments (
    notice_id uuid NOT NULL REFERENCES notices(id) ON DELETE CASCADE,
    file_id uuid NOT NULL UNIQUE REFERENCES file_objects(id) ON DELETE RESTRICT,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 9),
    PRIMARY KEY (notice_id, file_id)
);

CREATE TABLE school_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title_en text NOT NULL CHECK (title_en = btrim(title_en) AND title_en <> '' AND octet_length(title_en) <= 200),
    title_bn text NOT NULL DEFAULT '' CHECK (title_bn = btrim(title_bn) AND octet_length(title_bn) <= 600),
    description_en text NOT NULL DEFAULT '' CHECK (octet_length(description_en) <= 5000),
    description_bn text NOT NULL DEFAULT '' CHECK (octet_length(description_bn) <= 15000),
    kind text NOT NULL CHECK (kind IN ('holiday', 'exam', 'meeting', 'event', 'other')),
    starts_on date NOT NULL,
    ends_on date NOT NULL,
    start_time time,
    end_time time,
    location text NOT NULL DEFAULT '' CHECK (octet_length(location) <= 300),
    audience_roles text[] NOT NULL CHECK (cardinality(audience_roles) > 0 AND audience_roles <@ ARRAY['head_teacher','teacher','student','guardian','accountant']::text[]),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled')),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT school_events_dates_ck CHECK (ends_on >= starts_on AND ends_on - starts_on <= 366),
    CONSTRAINT school_events_times_ck CHECK ((start_time IS NULL) = (end_time IS NULL) AND (start_time IS NULL OR starts_on < ends_on OR end_time > start_time))
);
CREATE INDEX school_events_range_idx ON school_events(starts_on, ends_on) WHERE status <> 'draft';

CREATE TABLE school_event_targets (
    event_id uuid NOT NULL REFERENCES school_events(id) ON DELETE CASCADE,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    CONSTRAINT school_event_targets_key UNIQUE NULLS NOT DISTINCT (event_id, class_id, section_id)
);

INSERT INTO permissions(id, description) VALUES
 ('notices:read', 'Read notices addressed to the account'),
 ('notices:manage', 'Create, edit, publish and archive school notices'),
 ('events:read', 'Read the school events calendar for the account'),
 ('events:manage', 'Create, edit, publish and cancel school events')
ON CONFLICT(id) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN (VALUES ('notices:read'), ('events:read')) AS p(id)
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id) VALUES
 ('admin', 'notices:manage'), ('admin', 'events:manage'),
 ('head_teacher', 'notices:manage'), ('head_teacher', 'events:manage')
ON CONFLICT DO NOTHING;
