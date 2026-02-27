/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

const FALLBACK_VERSION = "0.0.0-dev"

export function getAppVersion(): string {
  if (typeof window === "undefined") {
    return FALLBACK_VERSION
  }

  const version = window.__QUI_VERSION__
  if (!version || version.trim() === "") {
    return FALLBACK_VERSION
  }

  return version
}
