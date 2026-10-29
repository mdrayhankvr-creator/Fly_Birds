-- School landing page: a short tagline under the school name and up to eight
-- photos for the public slideshow. Photos are stored like student photos:
-- in the database, re-encoded by the server (which drops embedded metadata),
-- as JPEG only. The slideshow is public by design; administrators choose
-- what goes in it.
ALTER TABLE school_settings
    ADD COLUMN tagline_en text NOT NULL DEFAULT '' CHECK (char_length(tagline_en) <= 160),
    ADD COLUMN tagline_bn text NOT NULL DEFAULT '' CHECK (char_length(tagline_bn) <= 160);

CREATE TABLE school_slides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 7),
    content_type text NOT NULL CHECK (content_type = 'image/jpeg'),
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 1048576),
    width integer NOT NULL CHECK (width BETWEEN 1 AND 2400),
    height integer NOT NULL CHECK (height BETWEEN 1 AND 2400),
    -- A content hash; image URLs carry it so browsers can cache them for good.
    version text NOT NULL CHECK (version ~ '^[0-9a-f]{16}$'),
    caption_en text NOT NULL DEFAULT '' CHECK (char_length(caption_en) <= 140),
    caption_bn text NOT NULL DEFAULT '' CHECK (char_length(caption_bn) <= 140),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- Deferred so a reorder can swap positions inside one transaction.
    CONSTRAINT school_slides_position_key UNIQUE (position) DEFERRABLE INITIALLY DEFERRED
);
