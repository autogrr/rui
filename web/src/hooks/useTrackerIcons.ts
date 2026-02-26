/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { api } from "@/lib/api"
import { useQuery } from "@tanstack/react-query"

/**
 * Hook for fetching all cached tracker icons
 * Returns a map of tracker hostnames to base64-encoded data URLs
 */
export function useTrackerIcons() {
  const query = useQuery<Record<string, string>>({
    queryKey: ["tracker-icons"],
    queryFn: () => api.getTrackerIcons(),
    staleTime: 60000, // 1 minute
    gcTime: 1800000, // Keep in cache for 30 minutes
    refetchInterval: 60000, // Refetch every 1 minute
    refetchIntervalInBackground: false,
    placeholderData: (previousData) => previousData,
  })

  return query
}
