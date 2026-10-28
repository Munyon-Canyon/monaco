# Mobile app

Feature code lives in two places. The model is `packages/mobile-core/Sources/MonacoCore/<Domain>/<Domain>Model.swift`. The view is `apps/mobile/Monaco/Features/<Domain>/<Domain>View.swift`. The rules are `docs/architecture/ios.md` and the recipe is `docs/how-to/mobile-feature.md`. A new screen starts from the `ios-feature` skill.

Screen work (colour, type, touch targets, accessibility and copy) uses the `ios-screen` skill.

## Rules

New rules go in `docs/agents/rules.yaml`. The list below is rendered from it.

<!-- BEGIN GENERATED rules:mobile-agents-md (docs/agents/rules.yaml) -->
- When the user says "test mobile" or runs `just test mobile`, run host `swift test` in `packages/mobile-core` (fast, deterministic; no iOS Simulator or `xcodebuild test`).
- Do not boot a sim for unit tests.
- Never `simctl erase`.
- Agent tap-through may use `.cursor/skills/ios-simslim-fast-qa/SKILL.md` or MobileBuildMCP with `--simulator-id` from `scripts/gold-sim-udid.sh` (the worktree's own sim in a lane; in primary fail-loud if `SIMSLIM_UDID` is unset; not used by human `just run`).
- Product UI: prefer toasts over main-UI popups or banners for action confirmations; never hyphenate wallet addresses; show human-readable stock names or symbols, not raw mint addresses or "xStock" branding.
- User-facing copy uses "cabal" (not club/group), and so do new API routes, Go types, tables and event subjects (`/v1/cabals/{id}`; see `docs/architecture/backend-platform.md#decided`).
- `monacoctl agents check` is stage 0. On darwin, an app or mobile-core change also runs the `xcode` row: build for testing, with no tests and no simulator; MonacoTests run in the hourly staging `ios` job. `just build mobile` compiles the app.
- Warnings are errors (#980). The app is Swift 6 with complete checking (#937). Format with `swift format` (#938).
- Call the backend only through `MonacoAPI`: enforced by `lint:scripts/check-legacy-growth.py`.
<!-- END GENERATED rules:mobile-agents-md -->
