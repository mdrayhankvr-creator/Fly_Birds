-- The school logo can be uploaded in School settings, not only given as a
-- web link. The upload is resized (at most 512 px on its longer side) and
-- compressed on the server: PNG when it has transparency, JPEG otherwise.
-- school_settings.logo_url then points at /api/school/logo?v=<version>, so
-- every page that shows the logo uses it unchanged, and documents read the
-- bytes from here instead of downloading them.
CREATE TABLE school_logo (
    id integer PRIMARY KEY CHECK (id = 1),
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg')),
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 524288),
    version text NOT NULL CHECK (version ~ '^[0-9a-f]{12}$'),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
