/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { cn } from "@/lib/utils";
import { useDroppable } from "@dnd-kit/core";

interface DropZoneProps {
  id: string
}

export function DropZone({ id }: DropZoneProps) {
  const { isOver, setNodeRef } = useDroppable({ id });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        "h-2 rounded-sm border border-transparent transition-colors",
        isOver && "border-primary/60 bg-primary/20"
      )}
      aria-hidden="true"
    />
  );
}
