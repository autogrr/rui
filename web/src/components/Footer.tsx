/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { Github } from "lucide-react";
import { Button } from "@/components/ui/button";

export const Footer = () => {
  return (
    <div className="flex items-center justify-between mt-6 px-1">
      <p className="text-[10px] text-muted-foreground/60">
        © {new Date().getFullYear() > 2025 ? `2025-${new Date().getFullYear()}` : new Date().getFullYear()} autobrr • AGPL-1.0-or-later
      </p>
      <Button
        variant="ghost"
        size="icon"
        className="h-5 w-5 text-muted-foreground/60 hover:text-muted-foreground"
        asChild
      >
        <a
          href="https://github.com/autogrr/rui"
          target="_blank"
          rel="noopener noreferrer"
          aria-label="View on GitHub"
        >
          <Github className="h-3 w-3" />
        </a>
      </Button>
    </div>
  );
};