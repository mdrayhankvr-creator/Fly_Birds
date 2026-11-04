-- School details are school settings.
--
-- The EIIN, the founding year, the head teacher's name, the address in Bangla
-- and the map link describe the school, not the public page, so they live in
-- school_settings next to the name, address, phone and email, where the
-- school settings API returns them and documents such as the marksheet read
-- them. The public page shows them from here. Values saved on the public page
-- (migration 1757) are moved over; its contact address, phone and email fill
-- the school's own only where those are still empty.
ALTER TABLE school_settings
    ADD COLUMN eiin text NOT NULL DEFAULT '' CHECK (eiin ~ '^([0-9]{6})?$'),
    ADD COLUMN founded_year smallint CHECK (founded_year IS NULL OR founded_year BETWEEN 1800 AND 2100),
    ADD COLUMN address_bn text NOT NULL DEFAULT '' CHECK (char_length(address_bn) <= 500),
    ADD COLUMN map_url text NOT NULL DEFAULT '' CHECK (map_url = '' OR (map_url ~ '^https://' AND char_length(map_url) <= 500)),
    ADD COLUMN head_teacher_name_en text NOT NULL DEFAULT '' CHECK (char_length(head_teacher_name_en) <= 160),
    ADD COLUMN head_teacher_name_bn text NOT NULL DEFAULT '' CHECK (char_length(head_teacher_name_bn) <= 160);

UPDATE school_settings s SET
    eiin = p.eiin,
    founded_year = p.founded_year,
    address_bn = p.contact_address_bn,
    map_url = p.map_url,
    head_teacher_name_en = p.head_name_en,
    head_teacher_name_bn = p.head_name_bn,
    address = CASE WHEN s.address = '' THEN p.contact_address_en ELSE s.address END,
    phone = CASE WHEN s.phone = '' AND p.contact_phone ~ '^\+?[0-9][0-9 -]{5,18}[0-9]$' AND char_length(p.contact_phone) <= 20 THEN p.contact_phone ELSE s.phone END,
    email = CASE WHEN s.email = '' THEN p.contact_email ELSE s.email END
FROM school_public_page p WHERE s.id = 1 AND p.id = 1;

ALTER TABLE school_public_page
    DROP COLUMN eiin,
    DROP COLUMN founded_year,
    DROP COLUMN head_name_en,
    DROP COLUMN head_name_bn,
    DROP COLUMN contact_address_en,
    DROP COLUMN contact_address_bn,
    DROP COLUMN contact_phone,
    DROP COLUMN contact_email,
    DROP COLUMN map_url;
