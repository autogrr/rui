/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { createFileRoute } from "@tanstack/react-router"
import { Setup } from "@/pages/Setup"

export const Route = createFileRoute("/setup")({
  component: Setup,
})