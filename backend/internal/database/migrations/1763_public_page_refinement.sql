-- Modern Bold content is edited per school. The other two designs ignore it.
ALTER TABLE school_public_page
    ADD COLUMN activities_en text NOT NULL DEFAULT '' CHECK (char_length(activities_en) <= 1000),
    ADD COLUMN activities_bn text NOT NULL DEFAULT '' CHECK (char_length(activities_bn) <= 1000),
    ADD COLUMN footer_description_en text NOT NULL DEFAULT '' CHECK (char_length(footer_description_en) <= 300),
    ADD COLUMN footer_description_bn text NOT NULL DEFAULT '' CHECK (char_length(footer_description_bn) <= 300),
    ADD COLUMN social_facebook text NOT NULL DEFAULT '' CHECK (char_length(social_facebook) <= 500),
    ADD COLUMN social_youtube text NOT NULL DEFAULT '' CHECK (char_length(social_youtube) <= 500),
    ADD COLUMN social_whatsapp text NOT NULL DEFAULT '' CHECK (char_length(social_whatsapp) <= 500),
    ADD COLUMN social_messenger text NOT NULL DEFAULT '' CHECK (char_length(social_messenger) <= 500),
    ADD COLUMN privacy_policy_en text NOT NULL DEFAULT '' CHECK (char_length(privacy_policy_en) <= 5000),
    ADD COLUMN privacy_policy_bn text NOT NULL DEFAULT '' CHECK (char_length(privacy_policy_bn) <= 5000),
    ADD COLUMN terms_en text NOT NULL DEFAULT '' CHECK (char_length(terms_en) <= 5000),
    ADD COLUMN terms_bn text NOT NULL DEFAULT '' CHECK (char_length(terms_bn) <= 5000);

CREATE TABLE school_public_features (
    position smallint PRIMARY KEY CHECK (position BETWEEN 1 AND 4),
    icon text NOT NULL CHECK (icon IN ('book', 'teacher', 'shield', 'users', 'school', 'grade', 'chart', 'globe')),
    title_en text NOT NULL CHECK (char_length(title_en) <= 100),
    title_bn text NOT NULL CHECK (char_length(title_bn) <= 100),
    description_en text NOT NULL CHECK (char_length(description_en) <= 240),
    description_bn text NOT NULL CHECK (char_length(description_bn) <= 240)
);

INSERT INTO school_public_features (position, icon, title_en, title_bn, description_en, description_bn) VALUES
    (1, 'teacher', 'Dedicated teachers', 'নিবেদিত শিক্ষক', 'Teachers guide every learner with care and attention.', 'যত্ন ও মনোযোগ দিয়ে শিক্ষকরা প্রতিটি শিক্ষার্থীকে পথ দেখান।'),
    (2, 'book', 'Joyful learning', 'আনন্দময় শিক্ষা', 'Lessons encourage curiosity, practice and confidence.', 'পাঠে কৌতূহল, অনুশীলন ও আত্মবিশ্বাসকে উৎসাহ দেওয়া হয়।'),
    (3, 'shield', 'A caring environment', 'যত্নশীল পরিবেশ', 'A welcoming place where children can learn and grow.', 'শিশুদের শেখা ও বেড়ে ওঠার জন্য আন্তরিক পরিবেশ।'),
    (4, 'users', 'School community', 'বিদ্যালয় পরিবার', 'Families and educators work together for students.', 'শিক্ষার্থীদের জন্য পরিবার ও শিক্ষকরা একসঙ্গে কাজ করেন।');
