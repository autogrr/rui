-- Migration 064: RBAC core — users, roles, permissions
-- Replaces the singleton `user` table with multi-user `users` table,
-- adds RBAC infrastructure (roles, permissions, mappings).

-- 1. Create the new multi-user `users` table with interned strings
CREATE TABLE IF NOT EXISTS users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    username_id       INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    password_hash_id  INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    display_name_id   INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    email_id          INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    is_active         BOOLEAN NOT NULL DEFAULT 1,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username_id);
CREATE INDEX IF NOT EXISTS idx_users_active   ON users(is_active);

CREATE TRIGGER IF NOT EXISTS trg_users_updated_at
AFTER UPDATE ON users BEGIN
    UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS users_view AS
SELECT u.id,
       sp_un.value  AS username,
       sp_pw.value  AS password_hash,
       sp_dn.value  AS display_name,
       sp_em.value  AS email,
       u.is_active,
       u.created_at,
       u.updated_at
FROM users u
JOIN string_pool sp_un ON u.username_id      = sp_un.id
JOIN string_pool sp_pw ON u.password_hash_id = sp_pw.id
LEFT JOIN string_pool sp_dn ON u.display_name_id = sp_dn.id
LEFT JOIN string_pool sp_em ON u.email_id        = sp_em.id;

-- 2. Migrate existing user from `user` table into `users`
INSERT INTO users (id, username_id, password_hash_id, created_at, updated_at)
SELECT u.id,
       (SELECT sp1.id FROM string_pool sp1 WHERE sp1.value = u.username),
       (SELECT sp2.id FROM string_pool sp2 WHERE sp2.value = u.password_hash),
       u.created_at, u.updated_at
FROM user u
WHERE EXISTS (SELECT 1 FROM string_pool WHERE value = u.username)
  AND EXISTS (SELECT 1 FROM string_pool WHERE value = u.password_hash);

-- If the user's strings weren't already in the pool, insert them first then retry
INSERT OR IGNORE INTO string_pool (value) SELECT username FROM user;
INSERT OR IGNORE INTO string_pool (value) SELECT password_hash FROM user;

INSERT OR IGNORE INTO users (id, username_id, password_hash_id, created_at, updated_at)
SELECT u.id,
       (SELECT sp1.id FROM string_pool sp1 WHERE sp1.value = u.username),
       (SELECT sp2.id FROM string_pool sp2 WHERE sp2.value = u.password_hash),
       u.created_at, u.updated_at
FROM user u;

-- 3. Roles
CREATE TABLE IF NOT EXISTS roles (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name_id        INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    description_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    is_system      BOOLEAN NOT NULL DEFAULT 0,
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE VIEW IF NOT EXISTS roles_view AS
SELECT r.id,
       sp_n.value AS name,
       sp_d.value AS description,
       r.is_system,
       r.created_at
FROM roles r
JOIN string_pool sp_n ON r.name_id = sp_n.id
LEFT JOIN string_pool sp_d ON r.description_id = sp_d.id;

-- Seed system roles
INSERT OR IGNORE INTO string_pool (value) VALUES ('admin');
INSERT OR IGNORE INTO string_pool (value) VALUES ('operator');
INSERT OR IGNORE INTO string_pool (value) VALUES ('viewer');
INSERT OR IGNORE INTO string_pool (value) VALUES ('Full system access');
INSERT OR IGNORE INTO string_pool (value) VALUES ('Manage instances and automations');
INSERT OR IGNORE INTO string_pool (value) VALUES ('Read-only access');

INSERT INTO roles (name_id, description_id, is_system)
VALUES (
    (SELECT id FROM string_pool WHERE value = 'admin'),
    (SELECT id FROM string_pool WHERE value = 'Full system access'),
    1
);
INSERT INTO roles (name_id, description_id, is_system)
VALUES (
    (SELECT id FROM string_pool WHERE value = 'operator'),
    (SELECT id FROM string_pool WHERE value = 'Manage instances and automations'),
    1
);
INSERT INTO roles (name_id, description_id, is_system)
VALUES (
    (SELECT id FROM string_pool WHERE value = 'viewer'),
    (SELECT id FROM string_pool WHERE value = 'Read-only access'),
    1
);

-- 4. Permissions
CREATE TABLE IF NOT EXISTS permissions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name_id        INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    description_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT
);

CREATE VIEW IF NOT EXISTS permissions_view AS
SELECT p.id,
       sp_n.value AS name,
       sp_d.value AS description
FROM permissions p
JOIN string_pool sp_n ON p.name_id = sp_n.id
LEFT JOIN string_pool sp_d ON p.description_id = sp_d.id;

-- Seed permissions
INSERT OR IGNORE INTO string_pool (value) VALUES ('users.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('users.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('instances.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('instances.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('automations.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('automations.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('crossseed.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('crossseed.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('indexers.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('indexers.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('notifications.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('notifications.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('backups.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('backups.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('settings.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('settings.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('apikeys.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('apikeys.view');
INSERT OR IGNORE INTO string_pool (value) VALUES ('licenses.manage');
INSERT OR IGNORE INTO string_pool (value) VALUES ('licenses.view');

INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'users.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'users.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'instances.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'instances.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'automations.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'automations.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'crossseed.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'crossseed.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'indexers.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'indexers.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'notifications.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'notifications.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'backups.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'backups.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'settings.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'settings.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'apikeys.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'apikeys.view'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'licenses.manage'));
INSERT INTO permissions (name_id) VALUES ((SELECT id FROM string_pool WHERE value = 'licenses.view'));

-- 5. Role-Permission mapping
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Admin gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT (SELECT r.id FROM roles r JOIN string_pool sp ON r.name_id = sp.id WHERE sp.value = 'admin'),
       p.id
FROM permissions p;

-- Operator gets all except users.manage and settings.manage
INSERT INTO role_permissions (role_id, permission_id)
SELECT (SELECT r.id FROM roles r JOIN string_pool sp ON r.name_id = sp.id WHERE sp.value = 'operator'),
       p.id
FROM permissions p
JOIN string_pool sp ON p.name_id = sp.id
WHERE sp.value NOT IN ('users.manage', 'settings.manage');

-- Viewer gets only *.view permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT (SELECT r.id FROM roles r JOIN string_pool sp ON r.name_id = sp.id WHERE sp.value = 'viewer'),
       p.id
FROM permissions p
JOIN string_pool sp ON p.name_id = sp.id
WHERE sp.value LIKE '%.view';

-- 6. User-Role assignment
CREATE TABLE IF NOT EXISTS user_roles (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);

-- Assign existing user the admin role
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
JOIN string_pool sp ON r.name_id = sp.id
WHERE sp.value = 'admin';

-- 7. Resource shares
CREATE TABLE IF NOT EXISTS resource_shares (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    resource_type_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    resource_id      INTEGER NOT NULL,
    owner_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_with      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id    INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(resource_type_id, resource_id, shared_with),
    CHECK(owner_id != shared_with)
);

CREATE INDEX IF NOT EXISTS idx_resource_shares_lookup   ON resource_shares(resource_type_id, shared_with);
CREATE INDEX IF NOT EXISTS idx_resource_shares_owner    ON resource_shares(owner_id);
CREATE INDEX IF NOT EXISTS idx_resource_shares_resource ON resource_shares(resource_type_id, resource_id);

CREATE VIEW IF NOT EXISTS resource_shares_view AS
SELECT rs.id,
       sp_rt.value AS resource_type,
       rs.resource_id,
       rs.owner_id,
       rs.shared_with,
       sp_pm.value AS permission,
       rs.created_at
FROM resource_shares rs
JOIN string_pool sp_rt ON rs.resource_type_id = sp_rt.id
JOIN string_pool sp_pm ON rs.permission_id    = sp_pm.id;

-- 8. Drop old user table
DROP TABLE IF EXISTS user;
DROP TRIGGER IF EXISTS update_user_updated_at;
