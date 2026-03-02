# GitHub Copilot Instructions

This repository uses an internal guidelines document named `AGENTS.md` that
provides comprehensive information for any AI assistant working on the codebase.
GitHub Copilot (or other AI tooling) should always open and read
`AGENTS.md` before suggesting changes or generating code. It contains critical
requirements about naming, licensing, project structure, testing, and other
conventions that must be followed exactly.

If your editor or Copilot configuration supports a custom instructions file,
point it to this document or include the contents of `AGENTS.md` so that the
assistant is aware of the policies described there.

> ⚠️ **Important:** Do **not** remove or modify `AGENTS.md` without explicit
> approval from the maintainers. It governs AI behaviour and legal licensing
> notices (AGPL) used across the project.
