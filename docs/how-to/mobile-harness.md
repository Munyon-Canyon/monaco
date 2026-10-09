# Debug sample harnesses

A rewire adds three things, each in its own file: a DEBUG fixture for the API types the screen reads, a harness entry that renders the screen without Privy or a backend, and a UI test that launches that entry. Nothing in `ContentView` or `MonacoApp` changes. The registry finds the entry.

## Add a fixture

Sample values are `static let sample` on the generated schema, plus named variants such as `sampleEchoed`. Put them in `packages/mobile-core/Sources/MonacoAPI/Fixtures/<Schema>+Sample.swift`, one schema per file, and wrap the file in `#if DEBUG`. Amounts are integer micros. Use an obvious placeholder for any id; do not copy a real wallet or mint address. `Ping+Sample.swift` is the first one.

Debug harnesses, `#Preview`s and host tests all read the same values. A Release build compiles none of the fixture files.

## Add a harness entry

Subclass `SampleHarnessEntry` in the feature's own file, under `#if DEBUG`. `root(arguments:auth:)` returns nil when the launch flag is absent, and otherwise the screen for that flag. Keep the flag string the existing screenshots and `MonacoUITests` already pass. There is no list to edit.

```swift
final class HomeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = HomeSampleScenario.matching(arguments) else { return nil }
        return AnyView(HomeSampleHarness(scenario: scenario, auth: auth))
    }
}
```

`SampleHarnessRegistry.requestedRoot(auth:)` asks every direct subclass. One match becomes the window's root. Two matches trip an assertion. No match falls through to `AuthGateView`, which is the login screen.

Launch flags stay exactly as they are (`-MonacoHomeSample populated`, `-MonacoCabalsTabSample`, `-MonacoDesignGallery`, `-MonacoChatSampleQA`, and the rest). `scripts/qa/screens.sh --check` fails in both directions. A new `-Monaco…Sample`, `-Monaco…Gallery` or `-…Harness` flag, or a new scenario case, with no line in `scripts/qa/sample-screens.txt` fails it. So does a line whose flag or scenario no harness reads, which is how a deleted harness leaves a gallery shot that shows the login screen. The Journeys CI job runs it.

## Flow scenarios

Every flow whose app status in `packages/flows/app/<id>.tsv` is `built` or `verified` gets one harness scenario per outcome other than `ok`. A `crash:` outcome is the `interrupted` scenario. The launch flag is `-MonacoFlow <id> <outcome>`, such as `-MonacoFlow 00 unauthorized`.

`cmd/gen flows` generates two of the three parts:

- `Flow<id>Scenario` in `packages/flows/Sources/MonacoFlows/Flow<id>Scenarios.gen.swift`, whose `matching(_:)` reads the flag. `import MonacoCore` brings it into the app.
- The lines between `# BEGIN generated flow scenarios` and `# END generated flow scenarios` in `scripts/qa/sample-screens.txt`. Never edit that block. `scripts/ci/ready.sh` fails when it is stale, and `scripts/check-gate-changes.py` warns on a hand edit inside it.

The harness entry is the only hand-written part. Put it in the flow's feature folder, next to its other entry, and drive the screen's model with a canned transport that answers each scenario. Flow 00's is `SystemPingFlowHarnessEntry` in `apps/mobile/Monaco/Features/SystemPing/SystemPingSampleHarness.swift`:

```swift
final class SystemPingFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow00Scenario.matching(arguments) else { return nil }
        let model = SystemPingModel.preview(answering: scenario)
        return AnyView(
            SystemPingRoute().destination(model: model)
                .task { await model.send(note: "Sample") }
        )
    }
}
```

`SystemPingModel.preview(answering:)` switches over every scenario with no `default:`, so a new backend outcome breaks the build until the app answers it. `scripts/qa/screens.sh --check` fails with `screens: -MonacoFlow <id> <outcome> has no harness entry` when no `SampleHarnessEntry` in `apps/mobile/Monaco` reads `Flow<id>Scenario.matching`.

## Drive it from a UI test

Set the flag on `XCUIApplication` and launch. The registry opens that harness; the test does not sign in and does not need a backend.

```swift
let app = XCUIApplication()
app.launchArguments = ["-MonacoHomeSample", "populated"]
app.launch()
```

The nightly runs the existing `MonacoUITests` classes with those same arguments. A new screen adds a class next to them and a manifest line, and leaves the other classes alone.

## What the gallery checks

`scripts/qa/screens.sh <sim udid> <Monaco.app> <dir>` shoots the manifest and fails on a line that did not draw its sample screen.

- **Drawn marker.** `SampleHarnessRegistry` writes the launch arguments to `monaco-sample-drawn` in the app's `tmp` directory when a harness root appears. The script deletes the marker, launches the line, and polls for it for `MONACO_QA_SCREEN_TIMEOUT` seconds (default 20). A line that never writes it is sign-in, the launch splash or a crash. It prints `screens: <name> never drew a sample screen (sign-in, splash or a crash)`, counts as failed and is not shot. `MONACO_QA_SCREEN_SETTLE` (default 3) is the wait after the marker, before the shot.
- **Crash check.** After each shot the script looks for `UIKitApplication:<bundle>` in `launchctl list`. A missing line prints `screens: <name> crashed after it drew` and counts as failed.
- **Identical shots.** After the loop, shots with the same `shasum` print `screens: <a> and <b> are the same picture`. It is a warning and does not change the exit code.
- **`-MonacoSampleScroll <center|bottom|0..1>`.** `SampleAppFrame` opens the tab screen scrolled to that anchor, such as `-MonacoGroupDetailSample populated -MonacoSampleScroll bottom`. Without the flag a harness's own anchor applies.
- **`-MonacoSampleAct`.** A harness whose state needs one action (Save, Send, Leave) performs it only when this flag is present (`SampleHarnessRegistry.actArgument`). `screens.sh` adds it to every line. Manifest lines never carry it and UI tests never pass it.
