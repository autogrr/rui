-- Copyright (c) 2026, the rui contributors.
-- SPDX-License-Identifier: AGPL-1.0-or-later
--
-- Migration 083: Recreate sessions table
-- Migration 078 dropped sessions as part of a planned JWT-based sessions
-- migration that was never completed. The SCS session manager (sqlite3store)
-- still requires this table, so recreate it here.

CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    data BLOB NOT NULL,
    expiry REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expiry);
