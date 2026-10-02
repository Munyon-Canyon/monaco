---
name: ios-feature
description: >-
  Adds a mobile screen on the generated client. Use when adding a screen, a new
  mobile feature, or a rewire of an existing one.
---

# iOS feature

Start from the generator, then swap the ping calls for the screen's operation. The rules are in [iOS architecture](../../../docs/architecture/ios.md). The recipe is [Add a mobile feature](../../../docs/how-to/mobile-feature.md). This page is only the generator.

Run from the repo root:

```bash
scripts/gen-mobile-feature.sh <Domain>
```

`<Domain>` is UpperCamelCase. The script refuses a name that is not, and it refuses a domain that already has a folder.

| File | What it is | Which tests run it |
| --- | --- | --- |
| `packages/mobile-core/Sources/MonacoCore/<Domain>/<Domain>Model.swift` | `@Observable` model | `swift test` |
| `packages/mobile-core/Tests/MonacoCoreTests/<Domain>ModelTests.swift` | Host tests | `swift test` |
| `apps/mobile/Monaco/Features/<Domain>/<Domain>View.swift` | SwiftUI binding | `just build mobile` |
| `apps/mobile/Monaco/Features/<Domain>/<Domain>Route.swift` | `AppRoute` for the screen | `just build mobile` |
| `apps/mobile/Monaco/Features/<Domain>/<Domain>SampleHarness.swift` | `SampleHarnessEntry` subclass | `MonacoTests` when a UI test launches it |
| `packages/mobile-core/Sources/MonacoAPI/Fixtures/<Domain>+Sample.swift` | DEBUG sample value | `swift test` |

The copy still calls `postSystemPing` and `getSystemPing`. Replace those with the screen's operation, then delete the ping-only assertions that no longer apply.

When the rewire deletes legacy code, lower the #941 `packages/mobile-core/legacy-baseline.tsv` row for every metric removed, and shrink the `feature-observable` allowlist row in `packages/mobile-core/Tests/MonacoCoreTests/RepoRulesAllowlist.txt` when an `@Observable` class leaves `Features/`.
