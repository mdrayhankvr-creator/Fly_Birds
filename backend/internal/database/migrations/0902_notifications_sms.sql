-- In-app notifications. source_key identifies the triggering fact (for example
-- one absence or one published exam), so dispatch can be repeated safely.
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category text NOT NULL CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary')),
    source_key text NOT NULL CHECK (source_key ~ '^[a-z_]+:[0-9a-zA-Z:_-]{1,200}$'),
    title_en text NOT NULL CHECK (octet_length(title_en) BETWEEN 1 AND 300),
    title_bn text NOT NULL DEFAULT '' CHECK (octet_length(title_bn) <= 900),
    body_en text NOT NULL DEFAULT '' CHECK (octet_length(body_en) <= 1000),
    body_bn text NOT NULL DEFAULT '' CHECK (octet_length(body_bn) <= 3000),
    link text NOT NULL DEFAULT '' CHECK (link = '' OR link ~ '^/[A-Za-z0-9/_?=&.-]{0,300}$'),
    urgent boolean NOT NULL DEFAULT false,
    read_at timestamptz,
    withdrawn_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notifications_source_key UNIQUE (user_id, source_key)
);
CREATE INDEX notifications_inbox_idx ON notifications(user_id, created_at DESC) WHERE withdrawn_at IS NULL;
CREATE INDEX notifications_unread_idx ON notifications(user_id) WHERE read_at IS NULL AND withdrawn_at IS NULL;

CREATE TABLE notification_preferences (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category text NOT NULL CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary')),
    in_app boolean NOT NULL DEFAULT true,
    sms boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, category)
);

-- Outbound SMS log. The only provider is a mock: 'sent_mock' means the mock
-- accepted the message; nothing is ever delivered to a phone.
CREATE TABLE sms_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phone text NOT NULL CHECK (phone ~ '^\+?[0-9]{8,15}$'),
    category text NOT NULL CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary')),
    source_key text NOT NULL,
    body text NOT NULL CHECK (octet_length(body) BETWEEN 1 AND 1000),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent_mock', 'failed')),
    provider text NOT NULL DEFAULT 'mock',
    provider_ref text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '' CHECK (octet_length(error) <= 500),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
    created_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    CONSTRAINT sms_messages_source_key UNIQUE (user_id, source_key)
);
CREATE INDEX sms_messages_queue_idx ON sms_messages(created_at) WHERE status = 'queued';
CREATE INDEX sms_messages_log_idx ON sms_messages(created_at DESC);

INSERT INTO permissions(id, description) VALUES
 ('notifications:read_self', 'Read and manage own notifications and preferences'),
 ('notifications:manage', 'Run notification dispatch and read the mock SMS log')
ON CONFLICT(id) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT id, 'notifications:read_self' FROM roles ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id) VALUES ('admin', 'notifications:manage') ON CONFLICT DO NOTHING;
