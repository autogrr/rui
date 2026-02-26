/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { useEffect, useState } from "react"

const STORAGE_KEY = "qui-titlebar-speeds-enabled"

export function usePersistedTitleBarSpeeds(defaultValue: boolean = false) {
  const [isEnabled, setIsEnabled] = useState<boolean>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored !== null) {
        const parsed = JSON.parse(stored)
        if (typeof parsed === "boolean") {
          return parsed
        }
      }
    } catch (error) {
      console.error("Failed to load title bar speed preference from localStorage:", error)
    }

    return defaultValue
  })

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(isEnabled))
    } catch (error) {
      console.error("Failed to save title bar speed preference to localStorage:", error)
    }
  }, [isEnabled])

  return [isEnabled, setIsEnabled] as const
}
