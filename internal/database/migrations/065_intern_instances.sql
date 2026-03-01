-- Migration 065: Rebuild instances with full string interning + owner_id
-- Current columns: id, name_id, host_id, username_id, password_encrypted(TEXT),
--   basic_username_id, basic_password_encrypted(TEXT), tls_skip_verify, sort_order,
--   is_active, has_local_filesystem_access, use_hardlinks, hardlink_base_dir(TEXT),
--   hardlink_dir_preset(TEXT), use_reflinks, fallback_to_regular_mode
-- Target: intern password_encrypted, basic_password_encrypted, hardlink_base_dir,
--   hardlink_dir_preset; add owner_id

-- Intern existing text values into string_pool
INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT password_encrypted FROM instances WHERE password_encrypted IS NOT NULL AND password_encrypted != '';

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT basic_password_encrypted FROM instances WHERE basic_password_encrypted IS NOT NULL AND basic_password_encrypted != '';

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT hardlink_base_dir FROM instances WHERE hardlink_base_dir IS NOT NULL AND hardlink_base_dir != '';

INSERT OR IGNORE INTO string_pool (value)
SELECT DISTINCT hardlink_dir_preset FROM instances WHERE hardlink_dir_preset IS NOT NULL AND hardlink_dir_preset != '';

-- Create new table
CREATE TABLE instances_new (
    id                            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id                      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id                       INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    host_id                       INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    username_id                   INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    password_encrypted_id         INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    basic_username_id             INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    basic_password_encrypted_id   INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    tls_skip_verify               BOOLEAN NOT NULL DEFAULT 0,
    sort_order                    INTEGER NOT NULL DEFAULT 0,
    is_active                     BOOLEAN NOT NULL DEFAULT 1,
    has_local_filesystem_access   BOOLEAN NOT NULL DEFAULT 0,
    use_hardlinks                 BOOLEAN NOT NULL DEFAULT 0,
    hardlink_base_dir_id          INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    hardlink_dir_preset_id        INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    use_reflinks                  BOOLEAN NOT NULL DEFAULT 0,
    fallback_to_regular_mode      BOOLEAN NOT NULL DEFAULT 0
);

-- Migrate data: assign owner_id from first user
INSERT INTO instances_new (
    id, owner_id, name_id, host_id, username_id,
    password_encrypted_id, basic_username_id, basic_password_encrypted_id,
    tls_skip_verify, sort_order, is_active, has_local_filesystem_access,
    use_hardlinks, hardlink_base_dir_id, hardlink_dir_preset_id,
    use_reflinks, fallback_to_regular_mode
)
SELECT
    i.id,
    (SELECT id FROM users LIMIT 1),
    i.name_id, i.host_id, i.username_id,
    COALESCE((SELECT sp.id FROM string_pool sp WHERE sp.value = i.password_encrypted),
             (SELECT id FROM string_pool WHERE value = '')),
    i.basic_username_id,
    (SELECT sp.id FROM string_pool sp WHERE sp.value = i.basic_password_encrypted),
    i.tls_skip_verify, i.sort_order, i.is_active, i.has_local_filesystem_access,
    i.use_hardlinks,
    COALESCE((SELECT sp.id FROM string_pool sp WHERE sp.value = i.hardlink_base_dir),
             (SELECT id FROM string_pool WHERE value = '')),
    COALESCE((SELECT sp.id FROM string_pool sp WHERE sp.value = i.hardlink_dir_preset),
             (SELECT id FROM string_pool WHERE value = '')),
    i.use_reflinks, i.fallback_to_regular_mode
FROM instances i;

-- Drop the view BEFORE dropping the table it depends on.
-- SQLite 3.25+ validates all views/triggers during ALTER TABLE RENAME,
-- so instances_view must not reference a dropped table at rename time.
DROP VIEW IF EXISTS instances_view;

-- Swap tables
DROP TABLE instances;
ALTER TABLE instances_new RENAME TO instances;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_instances_owner  ON instances(owner_id);
CREATE INDEX IF NOT EXISTS idx_instances_sort   ON instances(sort_order, id);
CREATE INDEX IF NOT EXISTS idx_instances_active ON instances(is_active, id);

-- Recreate view
DROP VIEW IF EXISTS instances_view;
CREATE VIEW instances_view AS
SELECT i.id, i.owner_id,
       sp_n.value AS name, sp_h.value AS host, sp_u.value AS username,
       sp_pe.value AS password_encrypted,
       sp_bu.value AS basic_username, sp_bp.value AS basic_password_encrypted,
       i.tls_skip_verify, i.sort_order, i.is_active, i.has_local_filesystem_access,
       i.use_hardlinks, sp_hd.value AS hardlink_base_dir, sp_hp.value AS hardlink_dir_preset,
       i.use_reflinks, i.fallback_to_regular_mode
FROM instances i
JOIN string_pool sp_n  ON i.name_id = sp_n.id
JOIN string_pool sp_h  ON i.host_id = sp_h.id
JOIN string_pool sp_u  ON i.username_id = sp_u.id
JOIN string_pool sp_pe ON i.password_encrypted_id = sp_pe.id
LEFT JOIN string_pool sp_bu ON i.basic_username_id = sp_bu.id
LEFT JOIN string_pool sp_bp ON i.basic_password_encrypted_id = sp_bp.id
JOIN string_pool sp_hd ON i.hardlink_base_dir_id = sp_hd.id
JOIN string_pool sp_hp ON i.hardlink_dir_preset_id = sp_hp.id;
