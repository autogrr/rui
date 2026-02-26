/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import App from "./App.tsx"
import { setupLaunchQueueConsumer } from "@/lib/launch-queue"
import "./index.css"

setupLaunchQueueConsumer()
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
)
