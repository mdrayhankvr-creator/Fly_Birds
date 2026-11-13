-- A short, section-scoped discussion belongs to each published diary.
CREATE TABLE diary_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    diary_id uuid NOT NULL REFERENCES class_diaries(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES diary_comments(id) ON DELETE RESTRICT,
    author_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    author_role text NOT NULL CHECK (author_role IN ('student', 'teacher')),
    body text NOT NULL CHECK (body = btrim(body) AND char_length(body) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    hidden_at timestamptz,
    hidden_by uuid REFERENCES users(id) ON DELETE SET NULL,
    hidden_reason text NOT NULL DEFAULT '' CHECK (char_length(hidden_reason) <= 500),
    deleted_at timestamptz,
    CONSTRAINT diary_comments_parent_role_ck CHECK ((parent_id IS NULL) = (author_role = 'student')),
    CONSTRAINT diary_comments_hide_ck CHECK ((hidden_at IS NULL AND hidden_by IS NULL AND hidden_reason = '') OR (hidden_at IS NOT NULL AND hidden_reason <> ''))
);
CREATE INDEX diary_comments_thread_idx ON diary_comments(diary_id, created_at, id);
CREATE INDEX diary_comments_parent_idx ON diary_comments(parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE diary_comment_reads (
    diary_id uuid NOT NULL REFERENCES class_diaries(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (diary_id, user_id)
);
