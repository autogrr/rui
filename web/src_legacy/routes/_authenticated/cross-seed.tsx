/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { CrossSeedPage } from "@/pages/CrossSeedPage"
import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

const crossSeedSearchSchema = z.object({
  tab: z.enum(["auto", "scan", "dir-scan", "rules", "blocklist"]).optional().catch(undefined),
})

export const Route = createFileRoute("/_authenticated/cross-seed")({
  validateSearch: crossSeedSearchSchema,
  component: CrossSeedRoute,
})

function CrossSeedRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()

  const handleTabChange = (tab: "auto" | "scan" | "dir-scan" | "rules" | "blocklist") => {
    navigate({
      search: { tab },
      replace: true,
    })
  }

  return (
    <CrossSeedPage
      activeTab={search.tab ?? "auto"}
      onTabChange={handleTabChange}
    />
  )
}
