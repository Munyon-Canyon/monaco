# Mobile app

Feature code lives in two places. The model is `packages/mobile-core/Sources/MonacoCore/<Domain>/<Domain>Model.swift`. The view is `apps/mobile/Monaco/Features/<Domain>/<Domain>View.swift`. The rules are `docs/architecture/ios.md` and the recipe is `docs/how-to/mobile-feature.md`. A new screen starts from the `ios-feature` skill.

Screen work (colour, type, touch targets, accessibility and copy) uses the `ios-screen` skill.

## Build and test

- `just test mobile` runs host `swift test` in `packages/mobile-core`.
- `monacoctl agents check` is stage 0. On darwin, an app or mobile-core change also runs the `xcode` row: build for testing, then `MonacoTests`.
- `just build mobile` compiles the app.

Warnings are errors (#980). The app is Swift 6 with complete checking (#937). Format with `swift format` (#938).

## API

Do not add to `MonacoAPIClient.swift` or `API/DTOs/`. Call the backend only through `packages/mobile-core/Sources/MonacoAPI`.

## Copy

Say "cabal". Show a stock by name or symbol, never a mint address and never "xStock". Confirm an action with a toast, not a popup or a banner.

## Simulator

Do not boot a simulator for unit tests. Never `simctl erase`. Tap through only with `.cursor/skills/ios-simslim-fast-qa/SKILL.md` or MobileBuildMCP, passing `--simulator-id` from `scripts/gold-sim-udid.sh`.
