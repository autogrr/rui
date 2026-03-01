-- =============================================================================
-- rui Schema v2: Multi-User RBAC + Universal String Interning + JWT Sessions
-- =============================================================================
-- DESIGN PRINCIPLES:
--   1. ZERO raw TEXT in data tables — every string lives in string_pool and is
--      referenced by integer FK. The ONLY table that stores TEXT values is
--      string_pool itself.
--   2. Views project human-readable columns by JOINing string_pool,
--      so read queries return ordinary strings and Go struct scanning is
--      unchanged.
--   3. Writes intern strings first (via InternStrings / InternStringNullable),
--      then INSERT/UPDATE with the returned integer IDs.
--   4. JWT sessions live in a SEPARATE database (sessions.db).
--
-- Conventions:
--   *_id (FK to string_pool)  = interned string reference
--   *_id (FK to another table)= foreign key (disambiguated by FK constraint)
--   owner_id                  = FK to users(id), identifies resource owner
--   *_encrypted_id            = interned AES-GCM ciphertext (base64 string)
--   BLOB                      = raw binary data (not interned)
--   TIMESTAMP                 = TEXT in ISO-8601 format (date, not a string)
-- =============================================================================

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- =============================================================================
-- INFRASTRUCTURE
-- =============================================================================

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- String interning pool: the single source of truth for ALL text values.
CREATE TABLE IF NOT EXISTS string_pool (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    value      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Sentinel values (always present)
INSERT OR IGNORE INTO string_pool (value) VALUES ('(unknown)');
INSERT OR IGNORE INTO string_pool (value) VALUES ('(unnamed)');
INSERT OR IGNORE INTO string_pool (value) VALUES ('');

-- =============================================================================
-- RBAC: USERS, ROLES, PERMISSIONS
-- =============================================================================

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

-- Roles
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

-- Permissions
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

-- Role-Permission mapping
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- User-Role assignment
CREATE TABLE IF NOT EXISTS user_roles (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);

-- =============================================================================
-- RESOURCE SHARING
-- =============================================================================

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

-- =============================================================================
-- LICENSING
-- =============================================================================

CREATE TABLE IF NOT EXISTS licenses (
    id                        INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id                  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_key_id            INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    product_name_id           INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    status_id                 INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    activated_at              DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at                DATETIME,
    last_validated            DATETIME DEFAULT CURRENT_TIMESTAMP,
    polar_customer_id_sid     INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    polar_product_id_sid      INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    polar_activation_id_sid   INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    username_id               INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    provider_id               INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    dodo_instance_id_sid      INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at                DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at                DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_licenses_status  ON licenses(status_id);
CREATE INDEX IF NOT EXISTS idx_licenses_product ON licenses(product_name_id);
CREATE INDEX IF NOT EXISTS idx_licenses_key     ON licenses(license_key_id);
CREATE INDEX IF NOT EXISTS idx_licenses_owner   ON licenses(owner_id);

CREATE VIEW IF NOT EXISTS licenses_view AS
SELECT l.id, l.owner_id,
       sp_lk.value AS license_key, sp_pn.value AS product_name, sp_st.value AS status,
       l.activated_at, l.expires_at, l.last_validated,
       sp_pci.value AS polar_customer_id, sp_ppi.value AS polar_product_id,
       sp_pai.value AS polar_activation_id, sp_un.value AS username,
       sp_pr.value AS provider, sp_di.value AS dodo_instance_id,
       l.created_at, l.updated_at
FROM licenses l
JOIN string_pool sp_lk ON l.license_key_id = sp_lk.id
JOIN string_pool sp_pn ON l.product_name_id = sp_pn.id
JOIN string_pool sp_st ON l.status_id = sp_st.id
LEFT JOIN string_pool sp_pci ON l.polar_customer_id_sid = sp_pci.id
LEFT JOIN string_pool sp_ppi ON l.polar_product_id_sid = sp_ppi.id
LEFT JOIN string_pool sp_pai ON l.polar_activation_id_sid = sp_pai.id
LEFT JOIN string_pool sp_un  ON l.username_id = sp_un.id
LEFT JOIN string_pool sp_pr  ON l.provider_id = sp_pr.id
LEFT JOIN string_pool sp_di  ON l.dodo_instance_id_sid = sp_di.id;

-- =============================================================================
-- QBITTORRENT INSTANCES
-- =============================================================================

CREATE TABLE IF NOT EXISTS instances (
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

CREATE INDEX IF NOT EXISTS idx_instances_owner  ON instances(owner_id);
CREATE INDEX IF NOT EXISTS idx_instances_sort   ON instances(sort_order, id);
CREATE INDEX IF NOT EXISTS idx_instances_active ON instances(is_active, id);

CREATE VIEW IF NOT EXISTS instances_view AS
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

-- =============================================================================
-- API KEYS
-- =============================================================================

CREATE TABLE IF NOT EXISTS api_keys (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash_id  INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    name_id      INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner ON api_keys(owner_id);

CREATE VIEW IF NOT EXISTS api_keys_view AS
SELECT a.id, a.owner_id, sp_kh.value AS key_hash, sp_n.value AS name,
       a.created_at, a.last_used_at
FROM api_keys a
JOIN string_pool sp_kh ON a.key_hash_id = sp_kh.id
JOIN string_pool sp_n  ON a.name_id = sp_n.id;

CREATE TABLE IF NOT EXISTS client_api_keys (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash_id     INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    client_name_id  INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    instance_id     INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at    TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_client_api_keys_hash     ON client_api_keys(key_hash_id);
CREATE INDEX IF NOT EXISTS idx_client_api_keys_owner    ON client_api_keys(owner_id);
CREATE INDEX IF NOT EXISTS idx_client_api_keys_instance ON client_api_keys(instance_id);

CREATE VIEW IF NOT EXISTS client_api_keys_view AS
SELECT c.id, c.owner_id, sp_kh.value AS key_hash, sp_cn.value AS client_name,
       c.instance_id, c.created_at, c.last_used_at
FROM client_api_keys c
JOIN string_pool sp_kh ON c.key_hash_id = sp_kh.id
JOIN string_pool sp_cn ON c.client_name_id = sp_cn.id;

-- =============================================================================
-- INSTANCE ERRORS
-- =============================================================================

CREATE TABLE IF NOT EXISTS instance_errors (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id      INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    error_type_id    INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    error_message_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    occurred_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_instance_errors_lookup ON instance_errors(instance_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_instance_errors_owner  ON instance_errors(owner_id);

CREATE TRIGGER IF NOT EXISTS cleanup_old_instance_errors
AFTER INSERT ON instance_errors BEGIN
    DELETE FROM instance_errors
    WHERE instance_id = NEW.instance_id
      AND id NOT IN (
          SELECT id FROM instance_errors
          WHERE instance_id = NEW.instance_id
          ORDER BY occurred_at DESC LIMIT 5);
END;

CREATE VIEW IF NOT EXISTS instance_errors_view AS
SELECT ie.id, ie.owner_id, ie.instance_id,
       sp_et.value AS error_type, sp_em.value AS error_message, ie.occurred_at
FROM instance_errors ie
JOIN string_pool sp_et ON ie.error_type_id = sp_et.id
JOIN string_pool sp_em ON ie.error_message_id = sp_em.id;

-- =============================================================================
-- INSTANCE BACKUPS
-- =============================================================================

CREATE TABLE IF NOT EXISTS instance_backup_settings (
    instance_id     INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled         BOOLEAN NOT NULL DEFAULT 0,
    hourly_enabled  BOOLEAN NOT NULL DEFAULT 0, daily_enabled BOOLEAN NOT NULL DEFAULT 0,
    weekly_enabled  BOOLEAN NOT NULL DEFAULT 0, monthly_enabled BOOLEAN NOT NULL DEFAULT 0,
    keep_hourly     INTEGER NOT NULL DEFAULT 0, keep_daily INTEGER NOT NULL DEFAULT 7,
    keep_weekly     INTEGER NOT NULL DEFAULT 4, keep_monthly INTEGER NOT NULL DEFAULT 12,
    include_categories BOOLEAN NOT NULL DEFAULT 1, include_tags BOOLEAN NOT NULL DEFAULT 1,
    custom_path_id  INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    expr_filter     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_backup_settings_owner ON instance_backup_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_backup_settings_updated
AFTER UPDATE ON instance_backup_settings BEGIN
    UPDATE instance_backup_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

CREATE VIEW IF NOT EXISTS instance_backup_settings_view AS
SELECT bs.instance_id, bs.owner_id, bs.enabled,
       bs.hourly_enabled, bs.daily_enabled, bs.weekly_enabled, bs.monthly_enabled,
       bs.keep_hourly, bs.keep_daily, bs.keep_weekly, bs.keep_monthly,
       bs.include_categories, bs.include_tags,
       COALESCE(sp_cp.value, '') AS custom_path,
       bs.expr_filter,
       bs.created_at, bs.updated_at
FROM instance_backup_settings bs
LEFT JOIN string_pool sp_cp ON bs.custom_path_id = sp_cp.id;

CREATE TABLE IF NOT EXISTS instance_backup_runs (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id                INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id             INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind_id                 INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    status_id               INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    requested_by_id         INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    requested_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at              TIMESTAMP, completed_at TIMESTAMP,
    archive_path_id         INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    manifest_path_id        INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    total_bytes             INTEGER NOT NULL DEFAULT 0,
    torrent_count           INTEGER NOT NULL DEFAULT 0,
    category_counts_json_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    categories_json_id      INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    tags_json_id            INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    error_message_id        INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_backup_runs_instance ON instance_backup_runs(instance_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_runs_owner    ON instance_backup_runs(owner_id);

CREATE VIEW IF NOT EXISTS instance_backup_runs_view AS
SELECT br.id, br.owner_id, br.instance_id,
       sp_k.value AS kind, sp_s.value AS status, sp_rb.value AS requested_by,
       br.requested_at, br.started_at, br.completed_at,
       sp_ap.value AS archive_path, sp_mp.value AS manifest_path,
       br.total_bytes, br.torrent_count,
       sp_ccj.value AS category_counts_json, sp_cj.value AS categories_json,
       sp_tj.value AS tags_json, sp_em.value AS error_message
FROM instance_backup_runs br
JOIN string_pool sp_k  ON br.kind_id = sp_k.id
JOIN string_pool sp_s  ON br.status_id = sp_s.id
JOIN string_pool sp_rb ON br.requested_by_id = sp_rb.id
LEFT JOIN string_pool sp_ap  ON br.archive_path_id = sp_ap.id
LEFT JOIN string_pool sp_mp  ON br.manifest_path_id = sp_mp.id
LEFT JOIN string_pool sp_ccj ON br.category_counts_json_id = sp_ccj.id
LEFT JOIN string_pool sp_cj  ON br.categories_json_id = sp_cj.id
LEFT JOIN string_pool sp_tj  ON br.tags_json_id = sp_tj.id
LEFT JOIN string_pool sp_em  ON br.error_message_id = sp_em.id;

CREATE TABLE IF NOT EXISTS instance_backup_items (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id               INTEGER NOT NULL REFERENCES instance_backup_runs(id) ON DELETE CASCADE,
    torrent_hash_id      INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    name_id              INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    category_id          INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    size_bytes           INTEGER NOT NULL,
    archive_rel_path_id  INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    infohash_v1_id       INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    infohash_v2_id       INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    tags_id              INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    torrent_blob_path_id INTEGER REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_backup_items_run  ON instance_backup_items(run_id);
CREATE INDEX IF NOT EXISTS idx_backup_items_hash ON instance_backup_items(torrent_hash_id);

CREATE VIEW IF NOT EXISTS instance_backup_items_view AS
SELECT bi.id, bi.run_id,
       sp_th.value AS torrent_hash, sp_n.value AS name, sp_c.value AS category,
       bi.size_bytes,
       sp_ar.value AS archive_rel_path, sp_v1.value AS infohash_v1, sp_v2.value AS infohash_v2,
       sp_t.value AS tags, sp_tb.value AS torrent_blob_path, bi.created_at
FROM instance_backup_items bi
JOIN string_pool sp_th ON bi.torrent_hash_id = sp_th.id
JOIN string_pool sp_n  ON bi.name_id = sp_n.id
LEFT JOIN string_pool sp_c  ON bi.category_id = sp_c.id
LEFT JOIN string_pool sp_ar ON bi.archive_rel_path_id = sp_ar.id
LEFT JOIN string_pool sp_v1 ON bi.infohash_v1_id = sp_v1.id
LEFT JOIN string_pool sp_v2 ON bi.infohash_v2_id = sp_v2.id
LEFT JOIN string_pool sp_t  ON bi.tags_id = sp_t.id
LEFT JOIN string_pool sp_tb ON bi.torrent_blob_path_id = sp_tb.id;

-- =============================================================================
-- TORRENT FILE CACHE (already fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS torrent_files_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    torrent_hash_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    file_index INTEGER NOT NULL,
    name_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    size INTEGER NOT NULL, progress REAL NOT NULL, priority INTEGER NOT NULL,
    is_seed INTEGER, piece_range_start INTEGER, piece_range_end INTEGER,
    availability REAL NOT NULL, cached_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id, torrent_hash_id, file_index)
);

CREATE INDEX IF NOT EXISTS idx_torrent_files_cache_lookup ON torrent_files_cache(instance_id, torrent_hash_id);
CREATE INDEX IF NOT EXISTS idx_torrent_files_cache_cached_at ON torrent_files_cache(cached_at);

CREATE VIEW IF NOT EXISTS torrent_files_cache_view AS
SELECT fc.id, fc.instance_id, sp_th.value AS torrent_hash, fc.file_index,
       sp_n.value AS name, fc.size, fc.progress, fc.priority, fc.is_seed,
       fc.piece_range_start, fc.piece_range_end, fc.availability, fc.cached_at
FROM torrent_files_cache fc
JOIN string_pool sp_th ON fc.torrent_hash_id = sp_th.id
JOIN string_pool sp_n  ON fc.name_id = sp_n.id;

CREATE TABLE IF NOT EXISTS torrent_files_sync (
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    torrent_hash_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    last_synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    torrent_progress REAL NOT NULL DEFAULT 0, file_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (instance_id, torrent_hash_id)
);

CREATE INDEX IF NOT EXISTS idx_torrent_files_sync_last_synced ON torrent_files_sync(last_synced_at);

CREATE VIEW IF NOT EXISTS torrent_files_sync_view AS
SELECT fs.instance_id, sp_th.value AS torrent_hash,
       fs.last_synced_at, fs.torrent_progress, fs.file_count
FROM torrent_files_sync fs
JOIN string_pool sp_th ON fs.torrent_hash_id = sp_th.id;

-- =============================================================================
-- EXTERNAL PROGRAMS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS external_programs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    path_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    args_template_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    enabled INTEGER NOT NULL DEFAULT 1, use_terminal INTEGER NOT NULL DEFAULT 1,
    path_mappings_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_external_programs_owner ON external_programs(owner_id);
CREATE INDEX IF NOT EXISTS idx_external_programs_enabled ON external_programs(enabled);

CREATE VIEW IF NOT EXISTS external_programs_view AS
SELECT ep.id, ep.owner_id, sp_n.value AS name, sp_p.value AS path,
       sp_at.value AS args_template, ep.enabled, ep.use_terminal,
       sp_pm.value AS path_mappings, ep.created_at, ep.updated_at
FROM external_programs ep
JOIN string_pool sp_n  ON ep.name_id = sp_n.id
JOIN string_pool sp_p  ON ep.path_id = sp_p.id
JOIN string_pool sp_at ON ep.args_template_id = sp_at.id
JOIN string_pool sp_pm ON ep.path_mappings_id = sp_pm.id;

-- =============================================================================
-- INSTANCE REANNOUNCE SETTINGS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS instance_reannounce_settings (
    instance_id INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    initial_wait_seconds INTEGER NOT NULL DEFAULT 15,
    reannounce_interval_seconds INTEGER NOT NULL DEFAULT 7,
    max_age_seconds INTEGER NOT NULL DEFAULT 600,
    aggressive INTEGER NOT NULL DEFAULT 0,
    monitor_all INTEGER NOT NULL DEFAULT 1,
    exclude_categories INTEGER NOT NULL DEFAULT 0,
    categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_tags INTEGER NOT NULL DEFAULT 0,
    tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_trackers INTEGER NOT NULL DEFAULT 0,
    trackers_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    max_retries INTEGER NOT NULL DEFAULT 50,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_reannounce_settings_owner ON instance_reannounce_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_reannounce_settings_updated
AFTER UPDATE ON instance_reannounce_settings BEGIN
    UPDATE instance_reannounce_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

CREATE VIEW IF NOT EXISTS instance_reannounce_settings_view AS
SELECT rs.instance_id, rs.owner_id, rs.enabled, rs.initial_wait_seconds,
       rs.reannounce_interval_seconds, rs.max_age_seconds, rs.aggressive,
       rs.monitor_all, rs.exclude_categories, sp_cj.value AS categories_json,
       rs.exclude_tags, sp_tj.value AS tags_json,
       rs.exclude_trackers, sp_trj.value AS trackers_json,
       rs.max_retries, rs.updated_at
FROM instance_reannounce_settings rs
JOIN string_pool sp_cj  ON rs.categories_json_id = sp_cj.id
JOIN string_pool sp_tj  ON rs.tags_json_id = sp_tj.id
JOIN string_pool sp_trj ON rs.trackers_json_id = sp_trj.id;

-- =============================================================================
-- INSTANCE CROSS-SEED COMPLETION SETTINGS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS instance_crossseed_completion_settings (
    instance_id INTEGER PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_categories_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    exclude_tags_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    indexer_ids_json_id INTEGER NOT NULL REFERENCES string_pool(id) ON DELETE RESTRICT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_crossseed_completion_settings_owner ON instance_crossseed_completion_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_crossseed_completion_settings_updated
AFTER UPDATE ON instance_crossseed_completion_settings BEGIN
    UPDATE instance_crossseed_completion_settings SET updated_at = CURRENT_TIMESTAMP WHERE instance_id = NEW.instance_id;
END;

CREATE VIEW IF NOT EXISTS instance_crossseed_completion_settings_view AS
SELECT cs.instance_id, cs.owner_id, cs.enabled,
       sp_cj.value AS categories_json, sp_tj.value AS tags_json,
       sp_ecj.value AS exclude_categories_json, sp_etj.value AS exclude_tags_json,
       sp_ij.value AS indexer_ids_json, cs.updated_at
FROM instance_crossseed_completion_settings cs
JOIN string_pool sp_cj  ON cs.categories_json_id = sp_cj.id
JOIN string_pool sp_tj  ON cs.tags_json_id = sp_tj.id
JOIN string_pool sp_ecj ON cs.exclude_categories_json_id = sp_ecj.id
JOIN string_pool sp_etj ON cs.exclude_tags_json_id = sp_etj.id
JOIN string_pool sp_ij  ON cs.indexer_ids_json_id = sp_ij.id;

-- =============================================================================
-- TORZNAB INDEXERS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS torznab_indexers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    base_url_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_id_string_id INTEGER REFERENCES string_pool(id),
    api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    backend_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled BOOLEAN DEFAULT 1, priority INTEGER DEFAULT 0, timeout_seconds INTEGER DEFAULT 30,
    capabilities_id INTEGER REFERENCES string_pool(id),
    last_test_at TIMESTAMP,
    last_test_status_id INTEGER REFERENCES string_pool(id),
    last_test_error_id INTEGER REFERENCES string_pool(id),
    limit_default INTEGER NOT NULL DEFAULT 100, limit_max INTEGER NOT NULL DEFAULT 100,
    basic_username_id INTEGER REFERENCES string_pool(id),
    basic_password_encrypted_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_torznab_indexers_owner ON torznab_indexers(owner_id);
CREATE INDEX IF NOT EXISTS idx_torznab_indexers_enabled ON torznab_indexers(enabled);
CREATE INDEX IF NOT EXISTS idx_torznab_indexers_priority ON torznab_indexers(priority DESC);

CREATE TRIGGER IF NOT EXISTS trg_torznab_indexers_updated
AFTER UPDATE ON torznab_indexers BEGIN
    UPDATE torznab_indexers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS torznab_indexers_view AS
SELECT ti.id, ti.owner_id, sp_n.value AS name, sp_bu.value AS base_url,
       sp_is.value AS indexer_id_string, sp_ak.value AS api_key_encrypted,
       sp_be.value AS backend, ti.enabled, ti.priority, ti.timeout_seconds,
       sp_cap.value AS capabilities, ti.last_test_at,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error,
       ti.limit_default, ti.limit_max,
       sp_bun.value AS basic_username, sp_bpe.value AS basic_password_encrypted,
       ti.created_at, ti.updated_at
FROM torznab_indexers ti
JOIN string_pool sp_n  ON ti.name_id = sp_n.id
JOIN string_pool sp_bu ON ti.base_url_id = sp_bu.id
JOIN string_pool sp_ak ON ti.api_key_encrypted_id = sp_ak.id
JOIN string_pool sp_be ON ti.backend_id = sp_be.id
LEFT JOIN string_pool sp_is  ON ti.indexer_id_string_id = sp_is.id
LEFT JOIN string_pool sp_cap ON ti.capabilities_id = sp_cap.id
LEFT JOIN string_pool sp_lts ON ti.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ti.last_test_error_id = sp_lte.id
LEFT JOIN string_pool sp_bun ON ti.basic_username_id = sp_bun.id
LEFT JOIN string_pool sp_bpe ON ti.basic_password_encrypted_id = sp_bpe.id;

CREATE TABLE IF NOT EXISTS torznab_indexer_capabilities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    capability_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, capability_type_id)
);
CREATE INDEX IF NOT EXISTS idx_torznab_capabilities_indexer ON torznab_indexer_capabilities(indexer_id);

CREATE VIEW IF NOT EXISTS torznab_indexer_capabilities_view AS
SELECT tc.id, tc.indexer_id, sp_ct.value AS capability_type, tc.created_at
FROM torznab_indexer_capabilities tc
JOIN string_pool sp_ct ON tc.capability_type_id = sp_ct.id;

CREATE TABLE IF NOT EXISTS torznab_indexer_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL, category_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    parent_category_id INTEGER, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_torznab_categories_indexer ON torznab_indexer_categories(indexer_id);

CREATE VIEW IF NOT EXISTS torznab_indexer_categories_view AS
SELECT tc.id, tc.indexer_id, tc.category_id, sp_cn.value AS category_name,
       tc.parent_category_id, tc.created_at
FROM torznab_indexer_categories tc
JOIN string_pool sp_cn ON tc.category_name_id = sp_cn.id;

CREATE TABLE IF NOT EXISTS torznab_indexer_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    error_message_id INTEGER NOT NULL REFERENCES string_pool(id),
    error_code_id INTEGER REFERENCES string_pool(id),
    occurred_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP, error_count INTEGER DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_indexer ON torznab_indexer_errors(indexer_id);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_occurred ON torznab_indexer_errors(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_torznab_errors_unresolved ON torznab_indexer_errors(indexer_id, resolved_at) WHERE resolved_at IS NULL;

CREATE VIEW IF NOT EXISTS torznab_indexer_errors_view AS
SELECT te.id, te.indexer_id, sp_em.value AS error_message, sp_ec.value AS error_code,
       te.occurred_at, te.resolved_at, te.error_count
FROM torznab_indexer_errors te
JOIN string_pool sp_em ON te.error_message_id = sp_em.id
LEFT JOIN string_pool sp_ec ON te.error_code_id = sp_ec.id;

CREATE TABLE IF NOT EXISTS torznab_indexer_cooldowns (
    indexer_id INTEGER PRIMARY KEY REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    resume_at TIMESTAMP NOT NULL, cooldown_seconds INTEGER NOT NULL,
    reason_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_torznab_cooldowns_resume ON torznab_indexer_cooldowns(resume_at);

CREATE TRIGGER IF NOT EXISTS trg_torznab_cooldowns_updated
AFTER UPDATE ON torznab_indexer_cooldowns FOR EACH ROW BEGIN
    UPDATE torznab_indexer_cooldowns SET updated_at = CURRENT_TIMESTAMP WHERE indexer_id = NEW.indexer_id;
END;

CREATE VIEW IF NOT EXISTS torznab_indexer_cooldowns_view AS
SELECT cd.indexer_id, cd.resume_at, cd.cooldown_seconds,
       sp_r.value AS reason, cd.created_at, cd.updated_at
FROM torznab_indexer_cooldowns cd
LEFT JOIN string_pool sp_r ON cd.reason_id = sp_r.id;

CREATE TABLE IF NOT EXISTS torznab_indexer_latency (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    operation_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    latency_ms INTEGER NOT NULL, success BOOLEAN NOT NULL,
    measured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_torznab_latency_indexer ON torznab_indexer_latency(indexer_id);
CREATE INDEX IF NOT EXISTS idx_torznab_latency_measured ON torznab_indexer_latency(measured_at DESC);

CREATE VIEW IF NOT EXISTS torznab_indexer_latency_view AS
SELECT tl.id, tl.indexer_id, sp_ot.value AS operation_type,
       tl.latency_ms, tl.success, tl.measured_at
FROM torznab_indexer_latency tl
JOIN string_pool sp_ot ON tl.operation_type_id = sp_ot.id;

CREATE VIEW IF NOT EXISTS torznab_indexer_latency_stats AS
SELECT tl.indexer_id, sp_ot.value AS operation_type,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN tl.success THEN 1 ELSE 0 END) AS successful_requests,
       AVG(tl.latency_ms) AS avg_latency_ms,
       MIN(tl.latency_ms) AS min_latency_ms,
       MAX(tl.latency_ms) AS max_latency_ms
FROM torznab_indexer_latency tl
JOIN string_pool sp_ot ON tl.operation_type_id = sp_ot.id
GROUP BY tl.indexer_id, sp_ot.value;

CREATE VIEW IF NOT EXISTS torznab_indexer_health AS
SELECT ti.id AS indexer_id, sp_n.value AS name,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error, ti.last_test_at,
       (SELECT COUNT(*) FROM torznab_indexer_errors te WHERE te.indexer_id = ti.id AND te.resolved_at IS NULL) AS unresolved_errors,
       (SELECT sp_em.value FROM torznab_indexer_errors te JOIN string_pool sp_em ON te.error_message_id = sp_em.id WHERE te.indexer_id = ti.id ORDER BY te.occurred_at DESC LIMIT 1) AS latest_error
FROM torznab_indexers ti
JOIN string_pool sp_n ON ti.name_id = sp_n.id
LEFT JOIN string_pool sp_lts ON ti.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ti.last_test_error_id = sp_lte.id;

-- =============================================================================
-- TORZNAB CACHES (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS torznab_torrent_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    cache_key_id INTEGER NOT NULL REFERENCES string_pool(id),
    guid_id INTEGER REFERENCES string_pool(id),
    download_url_id INTEGER REFERENCES string_pool(id),
    info_hash_id INTEGER REFERENCES string_pool(id),
    title_id INTEGER REFERENCES string_pool(id),
    size_bytes INTEGER,
    torrent_data BLOB NOT NULL,
    cached_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(indexer_id, cache_key_id)
);
CREATE INDEX IF NOT EXISTS idx_torznab_torrent_cache_last_used ON torznab_torrent_cache(last_used_at);

CREATE VIEW IF NOT EXISTS torznab_torrent_cache_view AS
SELECT tc.id, tc.indexer_id, sp_ck.value AS cache_key, sp_g.value AS guid,
       sp_du.value AS download_url, sp_ih.value AS info_hash, sp_t.value AS title,
       tc.size_bytes, tc.torrent_data, tc.cached_at, tc.last_used_at
FROM torznab_torrent_cache tc
JOIN string_pool sp_ck ON tc.cache_key_id = sp_ck.id
LEFT JOIN string_pool sp_g  ON tc.guid_id = sp_g.id
LEFT JOIN string_pool sp_du ON tc.download_url_id = sp_du.id
LEFT JOIN string_pool sp_ih ON tc.info_hash_id = sp_ih.id
LEFT JOIN string_pool sp_t  ON tc.title_id = sp_t.id;

CREATE TABLE IF NOT EXISTS torznab_search_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cache_key_id INTEGER NOT NULL REFERENCES string_pool(id),
    scope_id INTEGER NOT NULL REFERENCES string_pool(id),
    query_id INTEGER REFERENCES string_pool(id),
    categories_json_id INTEGER REFERENCES string_pool(id),
    indexer_ids_json_id INTEGER REFERENCES string_pool(id),
    indexer_matcher_id INTEGER NOT NULL REFERENCES string_pool(id),
    request_fingerprint_id INTEGER NOT NULL REFERENCES string_pool(id),
    response_data BLOB NOT NULL,
    total_results INTEGER NOT NULL DEFAULT 0,
    cached_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL, hit_count INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_torznab_search_cache_key ON torznab_search_cache(cache_key_id);
CREATE INDEX IF NOT EXISTS idx_torznab_search_cache_expires ON torznab_search_cache(expires_at);

CREATE VIEW IF NOT EXISTS torznab_search_cache_view AS
SELECT sc.id, sp_ck.value AS cache_key, sp_s.value AS scope, sp_q.value AS query,
       sp_cj.value AS categories_json, sp_ij.value AS indexer_ids_json,
       sp_im.value AS indexer_matcher, sp_rf.value AS request_fingerprint,
       sc.response_data, sc.total_results, sc.cached_at, sc.last_used_at,
       sc.expires_at, sc.hit_count
FROM torznab_search_cache sc
JOIN string_pool sp_ck ON sc.cache_key_id = sp_ck.id
JOIN string_pool sp_s  ON sc.scope_id = sp_s.id
JOIN string_pool sp_im ON sc.indexer_matcher_id = sp_im.id
JOIN string_pool sp_rf ON sc.request_fingerprint_id = sp_rf.id
LEFT JOIN string_pool sp_q  ON sc.query_id = sp_q.id
LEFT JOIN string_pool sp_cj ON sc.categories_json_id = sp_cj.id
LEFT JOIN string_pool sp_ij ON sc.indexer_ids_json_id = sp_ij.id;

CREATE TABLE IF NOT EXISTS torznab_search_cache_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    ttl_minutes INTEGER NOT NULL DEFAULT 1440,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- =============================================================================
-- CROSS-SEED (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS cross_seed_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT 0, run_interval_minutes INTEGER NOT NULL DEFAULT 120,
    start_paused BOOLEAN NOT NULL DEFAULT 1,
    category_id INTEGER REFERENCES string_pool(id),
    target_instance_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    target_indexer_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    max_results_per_run INTEGER NOT NULL DEFAULT 50,
    find_individual_episodes BOOLEAN NOT NULL DEFAULT 0,
    size_mismatch_tolerance_percent REAL NOT NULL DEFAULT 5.0,
    use_category_from_indexer BOOLEAN NOT NULL DEFAULT 0,
    run_external_program_id INTEGER REFERENCES external_programs(id) ON DELETE SET NULL,
    rss_automation_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    seeded_search_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    completion_search_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    inherit_source_tags BOOLEAN NOT NULL DEFAULT 0,
    use_cross_category_suffix BOOLEAN NOT NULL DEFAULT 1,
    skip_auto_resume_rss BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_seeded_search BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_completion BOOLEAN NOT NULL DEFAULT 0,
    skip_auto_resume_webhook BOOLEAN NOT NULL DEFAULT 0,
    rss_source_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_exclude_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    rss_source_exclude_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_exclude_categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    webhook_source_exclude_tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    skip_recheck BOOLEAN NOT NULL DEFAULT 0,
    skip_piece_boundary_safety_check BOOLEAN NOT NULL DEFAULT 1,
    use_custom_category BOOLEAN NOT NULL DEFAULT 0,
    custom_category_id INTEGER REFERENCES string_pool(id),
    use_cross_category_affix INTEGER NOT NULL DEFAULT 1,
    category_affix_mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    category_affix_id INTEGER NOT NULL REFERENCES string_pool(id),
    gazelle_enabled BOOLEAN NOT NULL DEFAULT 0,
    redacted_api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    orpheus_api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS trg_cross_seed_settings_updated
AFTER UPDATE ON cross_seed_settings BEGIN
    UPDATE cross_seed_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS cross_seed_settings_view AS
SELECT css.id, css.owner_id, css.enabled, css.run_interval_minutes, css.start_paused,
       sp_cat.value AS category, sp_tii.value AS target_instance_ids,
       sp_txi.value AS target_indexer_ids, css.max_results_per_run,
       css.find_individual_episodes, css.size_mismatch_tolerance_percent,
       css.use_category_from_indexer, css.run_external_program_id,
       sp_rat.value AS rss_automation_tags, sp_sst.value AS seeded_search_tags,
       sp_cst.value AS completion_search_tags, sp_wt.value AS webhook_tags,
       css.inherit_source_tags, css.use_cross_category_suffix,
       css.skip_auto_resume_rss, css.skip_auto_resume_seeded_search,
       css.skip_auto_resume_completion, css.skip_auto_resume_webhook,
       sp_rsc.value AS rss_source_categories, sp_rst.value AS rss_source_tags,
       sp_rsec.value AS rss_source_exclude_categories, sp_rset.value AS rss_source_exclude_tags,
       sp_wsc.value AS webhook_source_categories, sp_wst.value AS webhook_source_tags,
       sp_wsec.value AS webhook_source_exclude_categories, sp_wset.value AS webhook_source_exclude_tags,
       css.skip_recheck, css.skip_piece_boundary_safety_check, css.use_custom_category,
       sp_cc.value AS custom_category, css.use_cross_category_affix,
       sp_cam.value AS category_affix_mode, sp_ca.value AS category_affix,
       css.gazelle_enabled, sp_rak.value AS redacted_api_key_encrypted,
       sp_oak.value AS orpheus_api_key_encrypted, css.created_at, css.updated_at
FROM cross_seed_settings css
LEFT JOIN string_pool sp_cat ON css.category_id = sp_cat.id
JOIN string_pool sp_tii ON css.target_instance_ids_id = sp_tii.id
JOIN string_pool sp_txi ON css.target_indexer_ids_id = sp_txi.id
JOIN string_pool sp_rat ON css.rss_automation_tags_id = sp_rat.id
JOIN string_pool sp_sst ON css.seeded_search_tags_id = sp_sst.id
JOIN string_pool sp_cst ON css.completion_search_tags_id = sp_cst.id
JOIN string_pool sp_wt  ON css.webhook_tags_id = sp_wt.id
JOIN string_pool sp_rsc ON css.rss_source_categories_id = sp_rsc.id
JOIN string_pool sp_rst ON css.rss_source_tags_id = sp_rst.id
JOIN string_pool sp_rsec ON css.rss_source_exclude_categories_id = sp_rsec.id
JOIN string_pool sp_rset ON css.rss_source_exclude_tags_id = sp_rset.id
JOIN string_pool sp_wsc ON css.webhook_source_categories_id = sp_wsc.id
JOIN string_pool sp_wst ON css.webhook_source_tags_id = sp_wst.id
JOIN string_pool sp_wsec ON css.webhook_source_exclude_categories_id = sp_wsec.id
JOIN string_pool sp_wset ON css.webhook_source_exclude_tags_id = sp_wset.id
LEFT JOIN string_pool sp_cc ON css.custom_category_id = sp_cc.id
JOIN string_pool sp_cam ON css.category_affix_mode_id = sp_cam.id
JOIN string_pool sp_ca  ON css.category_affix_id = sp_ca.id
JOIN string_pool sp_rak ON css.redacted_api_key_encrypted_id = sp_rak.id
JOIN string_pool sp_oak ON css.orpheus_api_key_encrypted_id = sp_oak.id;

CREATE TABLE IF NOT EXISTS cross_seed_search_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL,
    categories_id INTEGER NOT NULL REFERENCES string_pool(id),
    tags_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_ids_id INTEGER NOT NULL REFERENCES string_pool(id),
    interval_seconds INTEGER NOT NULL DEFAULT 60, cooldown_minutes INTEGER NOT NULL DEFAULT 720,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIEW IF NOT EXISTS cross_seed_search_settings_view AS
SELECT csss.id, csss.owner_id, csss.instance_id,
       sp_c.value AS categories, sp_t.value AS tags, sp_ii.value AS indexer_ids,
       csss.interval_seconds, csss.cooldown_minutes, csss.created_at, csss.updated_at
FROM cross_seed_search_settings csss
JOIN string_pool sp_c  ON csss.categories_id = sp_c.id
JOIN string_pool sp_t  ON csss.tags_id = sp_t.id
JOIN string_pool sp_ii ON csss.indexer_ids_id = sp_ii.id;

CREATE TABLE IF NOT EXISTS cross_seed_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME,
    total_feed_items INTEGER NOT NULL DEFAULT 0, candidates_found INTEGER NOT NULL DEFAULT 0,
    torrents_added INTEGER NOT NULL DEFAULT 0, torrents_failed INTEGER NOT NULL DEFAULT 0,
    torrents_skipped INTEGER NOT NULL DEFAULT 0,
    message_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    results_json_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cross_seed_runs_started ON cross_seed_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_cross_seed_runs_owner ON cross_seed_runs(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_runs_view AS
SELECT csr.id, csr.owner_id, sp_tb.value AS triggered_by, sp_m.value AS mode,
       sp_s.value AS status, csr.started_at, csr.completed_at,
       csr.total_feed_items, csr.candidates_found, csr.torrents_added,
       csr.torrents_failed, csr.torrents_skipped,
       sp_msg.value AS message, sp_em.value AS error_message, sp_rj.value AS results_json,
       csr.created_at
FROM cross_seed_runs csr
JOIN string_pool sp_tb ON csr.triggered_by_id = sp_tb.id
JOIN string_pool sp_m  ON csr.mode_id = sp_m.id
JOIN string_pool sp_s  ON csr.status_id = sp_s.id
LEFT JOIN string_pool sp_msg ON csr.message_id = sp_msg.id
LEFT JOIN string_pool sp_em  ON csr.error_message_id = sp_em.id
LEFT JOIN string_pool sp_rj  ON csr.results_json_id = sp_rj.id;

CREATE TABLE IF NOT EXISTS cross_seed_feed_items (
    guid_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_id INTEGER NOT NULL REFERENCES torznab_indexers(id) ON DELETE CASCADE,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title_id INTEGER REFERENCES string_pool(id),
    first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_status_id INTEGER NOT NULL REFERENCES string_pool(id),
    last_run_id INTEGER REFERENCES cross_seed_runs(id) ON DELETE SET NULL,
    info_hash_id INTEGER REFERENCES string_pool(id),
    PRIMARY KEY (guid_id, indexer_id)
);
CREATE INDEX IF NOT EXISTS idx_cross_seed_feed_items_indexer ON cross_seed_feed_items(indexer_id);
CREATE INDEX IF NOT EXISTS idx_cross_seed_feed_items_owner ON cross_seed_feed_items(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_feed_items_view AS
SELECT sp_g.value AS guid, csfi.indexer_id, csfi.owner_id, sp_t.value AS title,
       csfi.first_seen_at, csfi.last_seen_at, sp_ls.value AS last_status,
       csfi.last_run_id, sp_ih.value AS info_hash
FROM cross_seed_feed_items csfi
JOIN string_pool sp_g ON csfi.guid_id = sp_g.id
LEFT JOIN string_pool sp_t ON csfi.title_id = sp_t.id
JOIN string_pool sp_ls ON csfi.last_status_id = sp_ls.id
LEFT JOIN string_pool sp_ih ON csfi.info_hash_id = sp_ih.id;

CREATE TABLE IF NOT EXISTS cross_seed_search_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME,
    total_torrents INTEGER NOT NULL DEFAULT 0, processed INTEGER NOT NULL DEFAULT 0,
    torrents_added INTEGER NOT NULL DEFAULT 0, torrents_failed INTEGER NOT NULL DEFAULT 0,
    torrents_skipped INTEGER NOT NULL DEFAULT 0,
    message_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    filters_json_id INTEGER REFERENCES string_pool(id),
    indexer_ids_json_id INTEGER REFERENCES string_pool(id),
    interval_seconds INTEGER NOT NULL DEFAULT 60, cooldown_minutes INTEGER NOT NULL DEFAULT 360,
    results_json_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cross_seed_search_runs_instance ON cross_seed_search_runs(instance_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_cross_seed_search_runs_owner ON cross_seed_search_runs(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_search_runs_view AS
SELECT cssr.id, cssr.owner_id, cssr.instance_id, sp_s.value AS status,
       cssr.started_at, cssr.completed_at, cssr.total_torrents, cssr.processed,
       cssr.torrents_added, cssr.torrents_failed, cssr.torrents_skipped,
       sp_msg.value AS message, sp_em.value AS error_message, sp_fj.value AS filters_json,
       sp_ij.value AS indexer_ids_json, cssr.interval_seconds, cssr.cooldown_minutes,
       sp_rj.value AS results_json, cssr.created_at
FROM cross_seed_search_runs cssr
JOIN string_pool sp_s ON cssr.status_id = sp_s.id
LEFT JOIN string_pool sp_msg ON cssr.message_id = sp_msg.id
LEFT JOIN string_pool sp_em ON cssr.error_message_id = sp_em.id
LEFT JOIN string_pool sp_fj ON cssr.filters_json_id = sp_fj.id
LEFT JOIN string_pool sp_ij ON cssr.indexer_ids_json_id = sp_ij.id
LEFT JOIN string_pool sp_rj ON cssr.results_json_id = sp_rj.id;

CREATE TABLE IF NOT EXISTS cross_seed_search_history (
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    torrent_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_searched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, torrent_hash_id)
);
CREATE INDEX IF NOT EXISTS idx_cross_seed_search_history_owner ON cross_seed_search_history(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_search_history_view AS
SELECT cssh.instance_id, sp_th.value AS torrent_hash, cssh.owner_id, cssh.last_searched_at
FROM cross_seed_search_history cssh
JOIN string_pool sp_th ON cssh.torrent_hash_id = sp_th.id;

CREATE TABLE IF NOT EXISTS cross_seed_blocklist (
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    infohash_id INTEGER NOT NULL REFERENCES string_pool(id),
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (instance_id, infohash_id)
);
CREATE INDEX IF NOT EXISTS idx_cross_seed_blocklist_owner ON cross_seed_blocklist(owner_id);

CREATE VIEW IF NOT EXISTS cross_seed_blocklist_view AS
SELECT csb.instance_id, sp_ih.value AS infohash, csb.owner_id, sp_n.value AS note, csb.created_at
FROM cross_seed_blocklist csb
JOIN string_pool sp_ih ON csb.infohash_id = sp_ih.id
JOIN string_pool sp_n  ON csb.note_id = sp_n.id;

-- =============================================================================
-- TRACKER CUSTOMIZATIONS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS tracker_customizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    domains_id INTEGER NOT NULL REFERENCES string_pool(id),
    included_in_stats_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tracker_customizations_owner ON tracker_customizations(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_tracker_customizations_updated
AFTER UPDATE ON tracker_customizations BEGIN
    UPDATE tracker_customizations SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS tracker_customizations_view AS
SELECT tc.id, tc.owner_id, sp_dn.value AS display_name, sp_d.value AS domains,
       sp_is.value AS included_in_stats, tc.created_at, tc.updated_at
FROM tracker_customizations tc
JOIN string_pool sp_dn ON tc.display_name_id = sp_dn.id
JOIN string_pool sp_d  ON tc.domains_id = sp_d.id
LEFT JOIN string_pool sp_is ON tc.included_in_stats_id = sp_is.id;

-- =============================================================================
-- DASHBOARD SETTINGS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS dashboard_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    section_visibility_id INTEGER NOT NULL REFERENCES string_pool(id),
    section_order_id INTEGER NOT NULL REFERENCES string_pool(id),
    section_collapsed_id INTEGER NOT NULL REFERENCES string_pool(id),
    tracker_breakdown_sort_column_id INTEGER REFERENCES string_pool(id),
    tracker_breakdown_sort_direction_id INTEGER REFERENCES string_pool(id),
    tracker_breakdown_items_per_page INTEGER DEFAULT 15,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS trg_dashboard_settings_updated
AFTER UPDATE ON dashboard_settings BEGIN
    UPDATE dashboard_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dashboard_settings_view AS
SELECT ds.id, ds.user_id,
       sp_sv.value AS section_visibility, sp_so.value AS section_order,
       sp_sc.value AS section_collapsed,
       sp_tbsc.value AS tracker_breakdown_sort_column,
       sp_tbsd.value AS tracker_breakdown_sort_direction,
       ds.tracker_breakdown_items_per_page, ds.created_at, ds.updated_at
FROM dashboard_settings ds
JOIN string_pool sp_sv ON ds.section_visibility_id = sp_sv.id
JOIN string_pool sp_so ON ds.section_order_id = sp_so.id
JOIN string_pool sp_sc ON ds.section_collapsed_id = sp_sc.id
LEFT JOIN string_pool sp_tbsc ON ds.tracker_breakdown_sort_column_id = sp_tbsc.id
LEFT JOIN string_pool sp_tbsd ON ds.tracker_breakdown_sort_direction_id = sp_tbsd.id;

-- =============================================================================
-- AUTOMATIONS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS automations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    tracker_pattern_id INTEGER NOT NULL REFERENCES string_pool(id),
    conditions_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1, sort_order INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER,
    free_space_source_id INTEGER REFERENCES string_pool(id),
    expr_filter_id INTEGER REFERENCES string_pool(id),
    dry_run INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_automations_owner ON automations(owner_id);
CREATE INDEX IF NOT EXISTS idx_automations_instance ON automations(instance_id, sort_order, id);

CREATE TRIGGER IF NOT EXISTS trg_automations_updated
AFTER UPDATE ON automations BEGIN
    UPDATE automations SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS automations_view AS
SELECT a.id, a.owner_id, a.instance_id,
       sp_n.value AS name, sp_tp.value AS tracker_pattern, sp_c.value AS conditions,
       a.enabled, a.sort_order, a.interval_seconds, sp_fs.value AS free_space_source,
       COALESCE(sp_ef.value, '') AS expr_filter,
       a.dry_run, a.created_at, a.updated_at
FROM automations a
JOIN string_pool sp_n  ON a.name_id = sp_n.id
JOIN string_pool sp_tp ON a.tracker_pattern_id = sp_tp.id
JOIN string_pool sp_c  ON a.conditions_id = sp_c.id
LEFT JOIN string_pool sp_fs ON a.free_space_source_id = sp_fs.id
LEFT JOIN string_pool sp_ef ON a.expr_filter_id       = sp_ef.id;

CREATE TABLE IF NOT EXISTS automation_activity (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    torrent_name_id INTEGER REFERENCES string_pool(id),
    tracker_domain_id INTEGER REFERENCES string_pool(id),
    action_id INTEGER NOT NULL REFERENCES string_pool(id),
    rule_id INTEGER,
    rule_name_id INTEGER REFERENCES string_pool(id),
    outcome_id INTEGER NOT NULL REFERENCES string_pool(id),
    reason_id INTEGER REFERENCES string_pool(id),
    details_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_automation_activity_instance ON automation_activity(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_automation_activity_owner ON automation_activity(owner_id);

CREATE VIEW IF NOT EXISTS automation_activity_view AS
SELECT aa.id, aa.owner_id, aa.instance_id,
       sp_h.value AS hash, sp_tn.value AS torrent_name, sp_td.value AS tracker_domain,
       sp_a.value AS action, aa.rule_id, sp_rn.value AS rule_name,
       sp_o.value AS outcome, sp_r.value AS reason, sp_d.value AS details, aa.created_at
FROM automation_activity aa
JOIN string_pool sp_h ON aa.hash_id = sp_h.id
JOIN string_pool sp_a ON aa.action_id = sp_a.id
JOIN string_pool sp_o ON aa.outcome_id = sp_o.id
LEFT JOIN string_pool sp_tn ON aa.torrent_name_id = sp_tn.id
LEFT JOIN string_pool sp_td ON aa.tracker_domain_id = sp_td.id
LEFT JOIN string_pool sp_rn ON aa.rule_name_id = sp_rn.id
LEFT JOIN string_pool sp_r  ON aa.reason_id = sp_r.id
LEFT JOIN string_pool sp_d  ON aa.details_id = sp_d.id;

-- =============================================================================
-- ORPHAN SCAN (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS orphan_scan_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0, grace_period_minutes INTEGER NOT NULL DEFAULT 10,
    ignore_paths_id INTEGER REFERENCES string_pool(id),
    scan_interval_hours INTEGER NOT NULL DEFAULT 24, max_files_per_run INTEGER NOT NULL DEFAULT 10000,
    auto_cleanup_enabled INTEGER NOT NULL DEFAULT 0, auto_cleanup_max_files INTEGER NOT NULL DEFAULT 100,
    preview_sort_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id)
);
CREATE INDEX IF NOT EXISTS idx_orphan_scan_settings_owner ON orphan_scan_settings(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_orphan_scan_settings_updated
AFTER UPDATE ON orphan_scan_settings BEGIN
    UPDATE orphan_scan_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS orphan_scan_settings_view AS
SELECT oss.id, oss.owner_id, oss.instance_id, oss.enabled, oss.grace_period_minutes,
       sp_ip.value AS ignore_paths, oss.scan_interval_hours, oss.max_files_per_run,
       oss.auto_cleanup_enabled, oss.auto_cleanup_max_files, sp_ps.value AS preview_sort,
       oss.created_at, oss.updated_at
FROM orphan_scan_settings oss
LEFT JOIN string_pool sp_ip ON oss.ignore_paths_id = sp_ip.id
JOIN string_pool sp_ps ON oss.preview_sort_id = sp_ps.id;

CREATE TABLE IF NOT EXISTS orphan_scan_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    scan_paths_id INTEGER REFERENCES string_pool(id),
    files_found INTEGER DEFAULT 0, files_deleted INTEGER DEFAULT 0,
    folders_deleted INTEGER DEFAULT 0, bytes_reclaimed INTEGER DEFAULT 0,
    truncated INTEGER NOT NULL DEFAULT 0,
    error_message_id INTEGER REFERENCES string_pool(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_orphan_scan_runs_instance ON orphan_scan_runs(instance_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_orphan_scan_runs_owner ON orphan_scan_runs(owner_id);

CREATE VIEW IF NOT EXISTS orphan_scan_runs_view AS
SELECT osr.id, osr.owner_id, osr.instance_id,
       sp_s.value AS status, sp_tb.value AS triggered_by, sp_sp.value AS scan_paths,
       osr.files_found, osr.files_deleted, osr.folders_deleted, osr.bytes_reclaimed,
       osr.truncated, sp_em.value AS error_message, osr.started_at, osr.completed_at
FROM orphan_scan_runs osr
JOIN string_pool sp_s  ON osr.status_id = sp_s.id
JOIN string_pool sp_tb ON osr.triggered_by_id = sp_tb.id
LEFT JOIN string_pool sp_sp ON osr.scan_paths_id = sp_sp.id
LEFT JOIN string_pool sp_em ON osr.error_message_id = sp_em.id;

CREATE TABLE IF NOT EXISTS orphan_scan_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES orphan_scan_runs(id) ON DELETE CASCADE,
    file_path_id INTEGER NOT NULL REFERENCES string_pool(id),
    file_size INTEGER NOT NULL, modified_at DATETIME,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id)
);
CREATE INDEX IF NOT EXISTS idx_orphan_scan_files_run ON orphan_scan_files(run_id);

CREATE VIEW IF NOT EXISTS orphan_scan_files_view AS
SELECT osf.id, osf.run_id, sp_fp.value AS file_path, osf.file_size, osf.modified_at,
       sp_s.value AS status, sp_em.value AS error_message
FROM orphan_scan_files osf
JOIN string_pool sp_fp ON osf.file_path_id = sp_fp.id
JOIN string_pool sp_s  ON osf.status_id = sp_s.id
LEFT JOIN string_pool sp_em ON osf.error_message_id = sp_em.id;

-- =============================================================================
-- ARR INSTANCES (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS arr_instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type_id INTEGER NOT NULL REFERENCES string_pool(id),
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    base_url_id INTEGER NOT NULL REFERENCES string_pool(id),
    api_key_encrypted_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled BOOLEAN DEFAULT 1, priority INTEGER DEFAULT 0, timeout_seconds INTEGER DEFAULT 15,
    last_test_at TIMESTAMP,
    last_test_status_id INTEGER REFERENCES string_pool(id),
    last_test_error_id INTEGER REFERENCES string_pool(id),
    basic_username_id INTEGER REFERENCES string_pool(id),
    basic_password_encrypted_id INTEGER REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_arr_instances_owner ON arr_instances(owner_id);
CREATE INDEX IF NOT EXISTS idx_arr_instances_enabled ON arr_instances(enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_arr_instances_type_base_url ON arr_instances(type_id, base_url_id);

CREATE TRIGGER IF NOT EXISTS trg_arr_instances_updated
AFTER UPDATE ON arr_instances BEGIN
    UPDATE arr_instances SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS arr_instances_view AS
SELECT ai.id, ai.owner_id, sp_t.value AS type, sp_n.value AS name,
       sp_bu.value AS base_url, sp_ak.value AS api_key_encrypted,
       ai.enabled, ai.priority, ai.timeout_seconds, ai.last_test_at,
       sp_lts.value AS last_test_status, sp_lte.value AS last_test_error,
       sp_bun.value AS basic_username, sp_bpe.value AS basic_password_encrypted,
       ai.created_at, ai.updated_at
FROM arr_instances ai
JOIN string_pool sp_t  ON ai.type_id = sp_t.id
JOIN string_pool sp_n  ON ai.name_id = sp_n.id
JOIN string_pool sp_bu ON ai.base_url_id = sp_bu.id
JOIN string_pool sp_ak ON ai.api_key_encrypted_id = sp_ak.id
LEFT JOIN string_pool sp_lts ON ai.last_test_status_id = sp_lts.id
LEFT JOIN string_pool sp_lte ON ai.last_test_error_id = sp_lte.id
LEFT JOIN string_pool sp_bun ON ai.basic_username_id = sp_bun.id
LEFT JOIN string_pool sp_bpe ON ai.basic_password_encrypted_id = sp_bpe.id;

CREATE TABLE IF NOT EXISTS arr_id_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    content_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    arr_instance_id INTEGER REFERENCES arr_instances(id) ON DELETE SET NULL,
    imdb_id_sid INTEGER REFERENCES string_pool(id),
    tmdb_id INTEGER, tvdb_id INTEGER, tvmaze_id INTEGER,
    is_negative BOOLEAN DEFAULT 0,
    cached_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, expires_at TIMESTAMP NOT NULL,
    UNIQUE(title_hash_id, content_type_id)
);
CREATE INDEX IF NOT EXISTS idx_arr_id_cache_lookup ON arr_id_cache(title_hash_id, content_type_id);
CREATE INDEX IF NOT EXISTS idx_arr_id_cache_expires ON arr_id_cache(expires_at);

CREATE VIEW IF NOT EXISTS arr_id_cache_view AS
SELECT aic.id, sp_th.value AS title_hash, sp_ct.value AS content_type,
       aic.arr_instance_id, sp_imdb.value AS imdb_id,
       aic.tmdb_id, aic.tvdb_id, aic.tvmaze_id, aic.is_negative,
       aic.cached_at, aic.expires_at
FROM arr_id_cache aic
JOIN string_pool sp_th ON aic.title_hash_id = sp_th.id
JOIN string_pool sp_ct ON aic.content_type_id = sp_ct.id
LEFT JOIN string_pool sp_imdb ON aic.imdb_id_sid = sp_imdb.id;

-- =============================================================================
-- LOG EXCLUSIONS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS log_exclusions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    patterns_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_log_exclusions_owner ON log_exclusions(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_log_exclusions_updated
AFTER UPDATE ON log_exclusions BEGIN
    UPDATE log_exclusions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS log_exclusions_view AS
SELECT le.id, le.owner_id, sp_p.value AS patterns, le.created_at, le.updated_at
FROM log_exclusions le
JOIN string_pool sp_p ON le.patterns_id = sp_p.id;

-- =============================================================================
-- DIRECTORY SCAN (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS dir_scan_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    match_mode_id INTEGER NOT NULL REFERENCES string_pool(id),
    size_tolerance_percent REAL NOT NULL DEFAULT 5.0, min_piece_ratio REAL NOT NULL DEFAULT 0.98,
    allow_partial INTEGER NOT NULL DEFAULT 0, skip_piece_boundary_safety_check INTEGER NOT NULL DEFAULT 1,
    start_paused INTEGER NOT NULL DEFAULT 1,
    category_id INTEGER REFERENCES string_pool(id), tags_id INTEGER REFERENCES string_pool(id),
    max_searchees_per_run INTEGER NOT NULL DEFAULT 0, max_searchee_age_days INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER IF NOT EXISTS trg_dir_scan_settings_updated
AFTER UPDATE ON dir_scan_settings BEGIN
    UPDATE dir_scan_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dir_scan_settings_view AS
SELECT dss.id, dss.owner_id, dss.enabled, sp_mm.value AS match_mode,
       dss.size_tolerance_percent, dss.min_piece_ratio, dss.allow_partial,
       dss.skip_piece_boundary_safety_check, dss.start_paused,
       sp_c.value AS category, sp_t.value AS tags,
       dss.max_searchees_per_run, dss.max_searchee_age_days, dss.created_at, dss.updated_at
FROM dir_scan_settings dss
JOIN string_pool sp_mm ON dss.match_mode_id = sp_mm.id
LEFT JOIN string_pool sp_c ON dss.category_id = sp_c.id
LEFT JOIN string_pool sp_t ON dss.tags_id = sp_t.id;

CREATE TABLE IF NOT EXISTS dir_scan_directories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path_id INTEGER NOT NULL REFERENCES string_pool(id),
    qbit_path_prefix_id INTEGER REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1,
    arr_instance_id INTEGER REFERENCES arr_instances(id) ON DELETE SET NULL,
    target_instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    scan_interval_minutes INTEGER NOT NULL DEFAULT 1440, last_scan_at DATETIME,
    category_id INTEGER REFERENCES string_pool(id), tags_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dir_scan_directories_owner ON dir_scan_directories(owner_id);

CREATE TRIGGER IF NOT EXISTS trg_dir_scan_directories_updated
AFTER UPDATE ON dir_scan_directories BEGIN
    UPDATE dir_scan_directories SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS dir_scan_directories_view AS
SELECT dsd.id, dsd.owner_id, sp_p.value AS path, sp_qp.value AS qbit_path_prefix,
       dsd.enabled, dsd.arr_instance_id, dsd.target_instance_id,
       dsd.scan_interval_minutes, dsd.last_scan_at,
       sp_c.value AS category, sp_t.value AS tags, dsd.created_at, dsd.updated_at
FROM dir_scan_directories dsd
JOIN string_pool sp_p ON dsd.path_id = sp_p.id
LEFT JOIN string_pool sp_qp ON dsd.qbit_path_prefix_id = sp_qp.id
LEFT JOIN string_pool sp_c ON dsd.category_id = sp_c.id
LEFT JOIN string_pool sp_t ON dsd.tags_id = sp_t.id;

CREATE TABLE IF NOT EXISTS dir_scan_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    triggered_by_id INTEGER NOT NULL REFERENCES string_pool(id),
    files_found INTEGER NOT NULL DEFAULT 0, files_skipped INTEGER NOT NULL DEFAULT 0,
    matches_found INTEGER NOT NULL DEFAULT 0, torrents_added INTEGER NOT NULL DEFAULT 0,
    error_message_id INTEGER REFERENCES string_pool(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_dir_scan_runs_directory ON dir_scan_runs(directory_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dir_scan_runs_owner ON dir_scan_runs(owner_id);

CREATE VIEW IF NOT EXISTS dir_scan_runs_view AS
SELECT dsr.id, dsr.owner_id, dsr.directory_id,
       sp_s.value AS status, sp_tb.value AS triggered_by,
       dsr.files_found, dsr.files_skipped, dsr.matches_found, dsr.torrents_added,
       sp_em.value AS error_message, dsr.started_at, dsr.completed_at
FROM dir_scan_runs dsr
JOIN string_pool sp_s  ON dsr.status_id = sp_s.id
JOIN string_pool sp_tb ON dsr.triggered_by_id = sp_tb.id
LEFT JOIN string_pool sp_em ON dsr.error_message_id = sp_em.id;

CREATE TABLE IF NOT EXISTS dir_scan_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    file_path_id INTEGER NOT NULL REFERENCES string_pool(id),
    file_size INTEGER NOT NULL, file_mod_time DATETIME NOT NULL, file_id BLOB,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    matched_torrent_hash_id INTEGER REFERENCES string_pool(id),
    matched_indexer_id INTEGER, last_processed_at DATETIME,
    UNIQUE(directory_id, file_path_id)
);
CREATE INDEX IF NOT EXISTS idx_dir_scan_files_fileid ON dir_scan_files(directory_id, file_id) WHERE file_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dir_scan_files_directory ON dir_scan_files(directory_id);

CREATE VIEW IF NOT EXISTS dir_scan_files_view AS
SELECT dsf.id, dsf.directory_id, sp_fp.value AS file_path, dsf.file_size,
       dsf.file_mod_time, dsf.file_id, sp_s.value AS status,
       sp_mth.value AS matched_torrent_hash, dsf.matched_indexer_id, dsf.last_processed_at
FROM dir_scan_files dsf
JOIN string_pool sp_fp ON dsf.file_path_id = sp_fp.id
JOIN string_pool sp_s  ON dsf.status_id = sp_s.id
LEFT JOIN string_pool sp_mth ON dsf.matched_torrent_hash_id = sp_mth.id;

CREATE TABLE IF NOT EXISTS dir_scan_run_injections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES dir_scan_runs(id) ON DELETE CASCADE,
    directory_id INTEGER NOT NULL REFERENCES dir_scan_directories(id) ON DELETE CASCADE,
    status_id INTEGER NOT NULL REFERENCES string_pool(id),
    searchee_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    torrent_name_id INTEGER NOT NULL REFERENCES string_pool(id),
    info_hash_id INTEGER NOT NULL REFERENCES string_pool(id),
    content_type_id INTEGER NOT NULL REFERENCES string_pool(id),
    indexer_name_id INTEGER REFERENCES string_pool(id),
    tracker_domain_id INTEGER REFERENCES string_pool(id),
    tracker_display_name_id INTEGER REFERENCES string_pool(id),
    link_mode_id INTEGER REFERENCES string_pool(id),
    save_path_id INTEGER REFERENCES string_pool(id),
    category_id INTEGER REFERENCES string_pool(id),
    tags_id INTEGER REFERENCES string_pool(id),
    error_message_id INTEGER REFERENCES string_pool(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dir_scan_injections_run ON dir_scan_run_injections(run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dir_scan_injections_directory ON dir_scan_run_injections(directory_id, created_at DESC);

CREATE VIEW IF NOT EXISTS dir_scan_run_injections_view AS
SELECT dsi.id, dsi.run_id, dsi.directory_id,
       sp_s.value AS status, sp_sn.value AS searchee_name, sp_tn.value AS torrent_name,
       sp_ih.value AS info_hash, sp_ct.value AS content_type, sp_in.value AS indexer_name,
       sp_td.value AS tracker_domain, sp_tdn.value AS tracker_display_name,
       sp_lm.value AS link_mode, sp_sp.value AS save_path,
       sp_c.value AS category, sp_t.value AS tags, sp_em.value AS error_message, dsi.created_at
FROM dir_scan_run_injections dsi
JOIN string_pool sp_s  ON dsi.status_id = sp_s.id
JOIN string_pool sp_sn ON dsi.searchee_name_id = sp_sn.id
JOIN string_pool sp_tn ON dsi.torrent_name_id = sp_tn.id
JOIN string_pool sp_ih ON dsi.info_hash_id = sp_ih.id
JOIN string_pool sp_ct ON dsi.content_type_id = sp_ct.id
LEFT JOIN string_pool sp_in  ON dsi.indexer_name_id = sp_in.id
LEFT JOIN string_pool sp_td  ON dsi.tracker_domain_id = sp_td.id
LEFT JOIN string_pool sp_tdn ON dsi.tracker_display_name_id = sp_tdn.id
LEFT JOIN string_pool sp_lm  ON dsi.link_mode_id = sp_lm.id
LEFT JOIN string_pool sp_sp  ON dsi.save_path_id = sp_sp.id
LEFT JOIN string_pool sp_c   ON dsi.category_id = sp_c.id
LEFT JOIN string_pool sp_t   ON dsi.tags_id = sp_t.id
LEFT JOIN string_pool sp_em  ON dsi.error_message_id = sp_em.id;

-- =============================================================================
-- NOTIFICATION TARGETS (fully interned)
-- =============================================================================

CREATE TABLE IF NOT EXISTS notification_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name_id INTEGER NOT NULL REFERENCES string_pool(id),
    url_id INTEGER NOT NULL REFERENCES string_pool(id),
    enabled INTEGER NOT NULL DEFAULT 1,
    event_types_id INTEGER NOT NULL REFERENCES string_pool(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notification_targets_owner ON notification_targets(owner_id);
CREATE INDEX IF NOT EXISTS idx_notification_targets_enabled ON notification_targets(enabled);

CREATE TRIGGER IF NOT EXISTS trg_notification_targets_updated
AFTER UPDATE ON notification_targets BEGIN
    UPDATE notification_targets SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE VIEW IF NOT EXISTS notification_targets_view AS
SELECT nt.id, nt.owner_id, sp_n.value AS name, sp_u.value AS url,
       nt.enabled, sp_et.value AS event_types, nt.created_at, nt.updated_at
FROM notification_targets nt
JOIN string_pool sp_n  ON nt.name_id = sp_n.id
JOIN string_pool sp_u  ON nt.url_id = sp_u.id
JOIN string_pool sp_et ON nt.event_types_id = sp_et.id;

-- =============================================================================
-- NOTE: JWT sessions stored in SEPARATE database (sessions.db).
-- Schema: see internal/auth/session_db.go
-- =============================================================================
