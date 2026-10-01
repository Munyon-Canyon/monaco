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

Launch flags stay exactly as they are (`-MonacoHomeSample populated`, `-MonacoCabalsTabSample`, `-MonacoDesignGallery`, `-MonacoChatSampleQA`, and the rest). `scripts/qa/screens.sh --check` fails when a new `-Monaco…Sample` or `-Monaco…Gallery` flag, or a new scenario case, has no line in `scripts/qa/sample-screens.txt`.

## Drive it from a UI test

Set the flag on `XCUIApplication` and launch. The registry opens that harness; the test does not sign in and does not need a backend.

```swift
let app = XCUIApplication()
app.launchArguments = ["-MonacoHomeSample", "populated"]
app.launch()
```

The nightly runs the existing `MonacoUITests` classes with those same arguments. A new screen adds a class next to them and a manifest line, and leaves the other classes alone.
