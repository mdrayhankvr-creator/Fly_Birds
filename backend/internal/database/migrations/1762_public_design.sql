-- Public page presentation is a school choice. Existing and new schools
-- start with the modern design; older API clients can omit the field.
ALTER TABLE school_public_page
    ADD COLUMN design_style text NOT NULL DEFAULT 'modern_bold'
        CHECK (design_style IN ('modern_bold', 'classic', 'minimal')),
    ADD COLUMN pass_rate smallint CHECK (pass_rate IS NULL OR pass_rate BETWEEN 0 AND 100);

-- Swatches are extracted from an uploaded logo. Empty means that the school's
-- manually selected theme colour remains the public-page fallback.
ALTER TABLE school_settings
    ADD COLUMN logo_palette_primary text NOT NULL DEFAULT '' CHECK (logo_palette_primary = '' OR logo_palette_primary ~ '^#[0-9A-Fa-f]{6}$'),
    ADD COLUMN logo_palette_accent text NOT NULL DEFAULT '' CHECK (logo_palette_accent = '' OR logo_palette_accent ~ '^#[0-9A-Fa-f]{6}$');
