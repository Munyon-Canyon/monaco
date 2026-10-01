# Add a mobile feature

Logic lives in host-testable `packages/mobile-core`. The SwiftUI view only binds. Copy `packages/mobile-core/Sources/MonacoCore/SystemPing/SystemPingModel.swift`. The rules below are also in [iOS architecture](../architecture/ios.md); this page is the recipe.

## Layout

| Piece | Path |
| --- | --- |
| Model | `packages/mobile-core/Sources/MonacoCore/<Domain>/<Domain>Model.swift` |
| View | `apps/mobile/Monaco/Features/<Domain>/<Domain>View.swift` |

The model imports neither SwiftUI nor Privy. The view calls no `APIClient` method of its own. Start a rewire with `scripts/gen-mobile-feature.sh <Domain>`, then swap the generated ping calls for the screen's operation. Putting the view on screen is `docs/how-to/mobile-navigation.md` (#942). A sample harness is `docs/how-to/mobile-harness.md` (#944). Those pages are not in the tree yet; a markdown link would fail the docs build.

## Model

`@Observable @MainActor public final class <Domain>Model`.

- Init takes `APIClient` and `any HintSource`. It reads no globals.
- `state` is `LoadState<Value>`: `.idle`, `.loading`, `.loaded(Value)`, `.failed(APIError)`. A refresh that starts from `.loaded` keeps that value until the next response arrives.
- `load()` does one `GET` through `api.read`.
- `observe()` iterates `hints.hints(matching:)` and calls `load()` on each match and on `.resync`. The view runs it from `.task`. The model is the only hint subscriber for the screen. Views never subscribe, and there is no hint-refresh view modifier.
- A screen that must coalesce bursts, or refresh only while visible, passes the stream to `HintRefresher` (#945) instead of calling `load()` per hint, and exposes `setVisible(_:)`, which the view calls through `.onScreenVisibilityChange`. System ping needs neither.
- A write uses one `IdempotentSubmission` owned by the model, through `api.submit`.
- Show failures with `ToastCopy.message(for:)`. Switch on `ProblemError.code` only where the flow branches.

One GET per screen. A hint means re-fetch, not a patch applied on the client.

## Tests

Host tests use `StubTransport` and `FakeHintStream` (`packages/mobile-core/Sources/MonacoTestSupport/FakeHintStream.swift`). `await FakeHintStream.send(_:)` delivers a hint to subscribers whose filter matches, including `.resync`. Take time from an injected clock; tests pass `TestClock` (#944). Fixtures live in `Sources/MonacoAPI/Fixtures/<Domain>+Sample.swift` (#944). Do not call live Jupiter.

`SystemPingModelTests` is the shape: one post with an `Idempotency-Key`, the same key after a transport error, one GET for `ping_echoed`, one GET for `.resync`, none for another key, and a problem body that becomes `.failed(.problem(...))` with `ToastCopy` returning the server message.

`SystemPingIntegrationTests` runs only when `MONACO_API_URL`, `MONACO_DEV_TOKEN` and `MONACO_DEV_USER` are set, against `just run backend`.

## Copy

User-facing strings say "cabal", not "club" or "group". Show a stock's name or symbol, never a mint address, and never "xStock". Confirm an action with a toast, not a popup or a banner. "Deposit" is inbound USDC. "Fund this cabal" is the sweep into a treasury.

## When you delete legacy code

Lower the #941 `packages/mobile-core/legacy-baseline.tsv` row for every metric you remove, in the same change. A count below the baseline fails until the row shrinks.
