CREATE TABLE roles (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE
);
INSERT INTO roles (id, name) VALUES
    ('admin', 'Administrator'),
    ('head_teacher', 'Head Teacher'),
    ('teacher', 'Teacher'),
    ('student', 'Student'),
    ('guardian', 'Guardian'),
    ('accountant', 'Accountant')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE permissions (
    id text PRIMARY KEY,
    description text NOT NULL
);
INSERT INTO permissions (id, description) VALUES
    ('auth:read_self', 'Read own account'),
    ('users:read', 'Read users'),
    ('users:create', 'Create users'),
    ('users:update', 'Update users'),
    ('users:delete', 'Delete users'),
    ('roles:read', 'Read roles and user role assignments'),
    ('permissions:read', 'Read permission matrix'),
    ('permissions:manage', 'Manage role permissions and user roles'),
    ('school_settings:read', 'Read school settings'),
    ('school_settings:update', 'Update school settings')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE role_permissions (
    role_id text NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    permission_id text NOT NULL REFERENCES permissions(id) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_id)
);
CREATE INDEX role_permissions_permission_idx ON role_permissions (permission_id);
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT id, 'auth:read_self' FROM roles WHERE id <> 'admin' ON CONFLICT DO NOTHING;

ALTER TABLE users ADD CONSTRAINT users_role_fk FOREIGN KEY (role) REFERENCES roles(id) ON UPDATE RESTRICT ON DELETE RESTRICT;
CREATE INDEX users_role_active_idx ON users (role, is_active);
