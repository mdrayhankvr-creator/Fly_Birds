-- Checkpoint 5: the public school page below the sign-in form.
--
-- Everything here is written by the administrator for anyone to read, and
-- holds nothing about individual students: no results, fees or records. Each
-- section is shown only when it has content. Photos are stored like the
-- slideshow's: re-encoded JPEG (which drops embedded metadata), with a small
-- copy for the gallery grid so slow connections load little.
CREATE TABLE school_public_page (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    about_en text NOT NULL DEFAULT '' CHECK (char_length(about_en) <= 3000),
    about_bn text NOT NULL DEFAULT '' CHECK (char_length(about_bn) <= 3000),
    founded_year smallint CHECK (founded_year IS NULL OR founded_year BETWEEN 1800 AND 2100),
    eiin text NOT NULL DEFAULT '' CHECK (eiin ~ '^([0-9]{6})?$'),
    -- Counts of active students and teachers, worked out when the page is read.
    show_counts boolean NOT NULL DEFAULT false,
    head_name_en text NOT NULL DEFAULT '' CHECK (char_length(head_name_en) <= 160),
    head_name_bn text NOT NULL DEFAULT '' CHECK (char_length(head_name_bn) <= 160),
    head_message_en text NOT NULL DEFAULT '' CHECK (char_length(head_message_en) <= 3000),
    head_message_bn text NOT NULL DEFAULT '' CHECK (char_length(head_message_bn) <= 3000),
    head_photo bytea CHECK (head_photo IS NULL OR octet_length(head_photo) BETWEEN 1 AND 524288),
    head_photo_version text NOT NULL DEFAULT '' CHECK (head_photo_version ~ '^([0-9a-f]{12})?$'),
    contact_address_en text NOT NULL DEFAULT '' CHECK (char_length(contact_address_en) <= 300),
    contact_address_bn text NOT NULL DEFAULT '' CHECK (char_length(contact_address_bn) <= 300),
    contact_phone text NOT NULL DEFAULT '' CHECK (char_length(contact_phone) <= 40),
    contact_email text NOT NULL DEFAULT '' CHECK (char_length(contact_email) <= 254),
    -- A link to a map (https only); the page shows a link, never an embedded map.
    map_url text NOT NULL DEFAULT '' CHECK (map_url = '' OR (map_url ~ '^https://' AND char_length(map_url) <= 500)),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO school_public_page (id) VALUES (1);

CREATE TABLE school_gallery (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 47),
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 1048576),
    thumb bytea NOT NULL CHECK (octet_length(thumb) BETWEEN 1 AND 131072),
    width integer NOT NULL CHECK (width BETWEEN 1 AND 1600),
    height integer NOT NULL CHECK (height BETWEEN 1 AND 1600),
    version text NOT NULL CHECK (version ~ '^[0-9a-f]{16}$'),
    caption_en text NOT NULL DEFAULT '' CHECK (char_length(caption_en) <= 140),
    caption_bn text NOT NULL DEFAULT '' CHECK (char_length(caption_bn) <= 140),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT school_gallery_position_key UNIQUE (position) DEFERRABLE INITIALLY DEFERRED
);

-- A notice appears on the public page only when its author ticks this.
ALTER TABLE notices ADD COLUMN show_public boolean NOT NULL DEFAULT false;
CREATE INDEX notices_public_idx ON notices (publish_at DESC) WHERE show_public AND status = 'published';
