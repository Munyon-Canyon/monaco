# iOS app

**Status:** Proposed 2026-09-30. Source of truth for the app, the way [backend-platform.md](backend-platform.md) is for the backend. [How to add a feature](../how-to/mobile-feature.md) is the recipe and does not restate this page.

## Ownership

MonacoCore owns product rules, money and address formatting, copy ("cabal", "deposit" versus "fund this cabal", stock names) and every API call. `swift test` covers that code. A `<Domain>Model` in MonacoCore (`@Observable @MainActor`) owns screen state. The SwiftUI view in `Features/<Domain>/` owns layout and binding only. A view that formats USDC, decodes JSON or builds a provider URL is a bug.

## State

New state is `@Observable`. `ObservableObject` is legacy: the 3 remaining classes convert when a ticket touches them, and #938's `observable-object` rule fails a new one.

## Navigation

One `NavigationStack` per tab, and `Hashable` routes that conform to `AppRoute`, one type per feature (#942). The how-to is [mobile-navigation.md](../how-to/mobile-navigation.md).

## Boundary

The app calls the Monaco API only, never Jupiter, Solana RPC, xStocks or Pyth. #938's `external-host` and `urlsession` rules enforce it.

## Concurrency

Swift 6 language mode with complete checking (#937). The app target's default actor isolation is `MainActor`.

## Screens

Screen work (colour, type, touch targets, accessibility and copy) follows the `ios-screen` skill in `.claude/skills/ios-screen/SKILL.md`.

## Not doing

No Clean Architecture use-case layer, MVC, TCA, reducers or coordinator framework. MonacoCore is the boundary. A second layer is a second paved path.

## Rules

`RepoRulesTests` (#938 and #943). A failing message can point at this table.

| Rule | Bans | Why |
| --- | --- | --- |
| `copy` | In `Text(`, `Button(`, `Label(`, `.navigationTitle(`, `String(localized:` and `ToastCopy`: `xStock`, the whole words `club` or `group`, and a base58 string of 32 to 44 characters | Product copy. A mint or "xStock" is not a name a member should see |
| `fixture-address` | A base58 string of 32 to 44 characters under `Sources/MonacoAPI/Fixtures/` | Fixtures use placeholders, not real addresses |
| `print` | `print(` under `apps/mobile/Monaco` or `packages/mobile-core/Sources`, including inside `#if DEBUG` | Logs go through `Logger` |
| `test-sleep` | `Task.sleep`, `asyncAfter` or `wait(for:` under `apps/mobile/MonacoTests` or `packages/mobile-core/Tests` | A sleep is a flake. Wait on the condition |
| `wall-clock` | `Date()`, `Date.now`, `ContinuousClock()` or `ContinuousClock.now` in `Sources/MonacoCore`, except a file named `*Clock.swift` | Code takes an injected clock |
| `float-money` | `Double(` or `Decimal(` in a file that mentions `micros`, and anywhere under `Sources/MonacoCore/<Domain>/` except `Networking/` | Money stays integer micros until a formatter renders it |
| `urlsession` | `URLSession` outside `Sources/MonacoAPI/` | The generated client is the only HTTP stack |
| `unchecked-sendable` | A new `@unchecked Sendable` | Shrink-only. Isolation should be explicit |
| `nonisolated-unsafe` | A new `nonisolated(unsafe)` | Shrink-only |
| `lint-directive` | `swiftlint:` or `swift-format-ignore` in a Swift file under `apps/mobile` or `packages/mobile-core` | A disable comment would also hide the comment ban |
| `observable-object` | `ObservableObject` or `@Published` in the app or MonacoCore | New screen state is an `@Observable` model |
| `external-host` | A string containing `jup.ag`, `api.xstocks.fi`, `pyth.network` or `solana.com` in the app or MonacoCore | The app talks to the Monaco API only. Explorer links on `solscan.io` stay |
| `feature-construct` | `APIClient(` or `HintStream(` under `Features/` outside `Features/Shell/`. A `#Preview` block is skipped | Features receive a client. They do not construct one. Previews may stub one |
| `view-hints` | `.hints(matching:` under `Features/` | The model subscribes. The view does not |
| `feature-observable` | `@Observable` under `Features/` outside `Features/Shell/` and `Features/Debug/` | Screen state is a `<Domain>Model` in MonacoCore. Legacy rows shrink when a rewire touches them |
| `feature-decode` | `JSONDecoder` under `Features/` | Decoding belongs to the generated client |
| `core-swiftui` | `import SwiftUI` or `canImport(SwiftUI)` in `Sources/MonacoCore` | MonacoCore stays host-testable and free of UI |
| `raw-color` | `Color(hex:`, `Color(red:` or `UIColor(red:` under `apps/mobile/Monaco` outside `Design/` | Colours are `MonacoTheme` tokens, which adapt to light and dark mode. Shrink-only |
| `fixed-font` | `.system(size:` under `apps/mobile/Monaco` outside `Design/` | A fixed point size does not scale with Dynamic Type. Shrink-only |

## Log

- 2026-09-30: Wrote the page for the system-ping reference feature (#943).
- 2026-10-01: Named the `ios-screen` skill for screen work and added the `raw-color` and `fixed-font` rules (#1032).
