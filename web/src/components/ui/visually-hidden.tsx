/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import * as React from "react"
import { cn } from "@/lib/utils"

type VisuallyHiddenProps = React.HTMLAttributes<HTMLSpanElement>

function VisuallyHidden({ className, ...props }: VisuallyHiddenProps) {
  return (
    <span
      className={cn(
        "absolute h-px w-px overflow-hidden whitespace-nowrap border-0 p-0 [clip:rect(0,0,0,0)]",
        className
      )}
      {...props}
    />
  )
}

export { VisuallyHidden }
