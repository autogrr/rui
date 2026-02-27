/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { InstanceBackups } from "@/pages/InstanceBackups"
import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/_authenticated/backups")({
  component: BackupsRoute,
})

function BackupsRoute() {
  return <InstanceBackups />
}
