# The XCUITest

Two files per journey in `apps/mobile/MonacoUITests/Journeys/`. The target compiles any file in that folder. `SignInJourney.swift` and `SignInJourneyUITests.swift` are the pattern to copy; `JourneySupport.swift` has the shared pieces named below.

## `<Journey>Journey.swift`: the steps

- An `enum` with one static function per scenario or per actor's part of a scenario. Its `id` and `version` come from the generated steps file, `<X>JourneySteps`, not from a hand-typed stamp.
- New and updated journeys use the generated steps. Run `scripts/qa/journey.py gen <journey>` after every change to a step table; it writes `apps/mobile/MonacoUITests/Journeys/Generated/<X>JourneySteps.swift` (id, version, and one `JourneyStep` per doc row: `id`, `name`, `actor`, `seconds` from the "within N s" of the Expect or 15, and `copy`, the Expect's quoted strings in order). `journey.py check` fails when that file is stale. Never edit it by hand.
- Every step in the journey doc is one `recorder.step(S.s1p4) { … }`, where `private typealias S = <X>JourneySteps` and `s1p4` is S1.4. Waits use `S.s1p4.seconds` and expected copy uses `S.s1p4.copy[0]`; a quote with a placeholder uses `S.s1p2.filled(4, ["QA.run": run])`. Failure messages start with `\(S.s1p4.id)`. The runner reads timings and the failing step from these. The string form `recorder.step("P1", "…")` stays for precondition steps and journeys not migrated yet.
- Waits poll: `waitForExistence(timeout:)` or `app.waitForFirst(of:timeout:)`, with the seconds the journey doc gives; prefer `JourneyDiagnostics.wait(for:in:timeout:step:)`, which stops at once on a `-failed` or `-error` identifier (or a system alert) and names it and the nearest on-screen identifiers.
- Every assertion message starts with the step id and states what was expected.
- Swift files carry no comments in this repo (`no_comments` in `.swiftlint.yml`): a name, a step label or an assertion message says what a comment would.
- A step function never launches, relaunches or signs in on its own account. The session does that, so one app session runs the whole journey. A step the doc writes as `relaunch` calls `app.terminate()` and `app.launch()`.

## `<Journey>JourneyUITests.swift`: one session

The whole journey is one test method, `testJourney`, and the runner runs it in one `xcodebuild` call on one simulator. Each scenario continues from where the one before it left off, so a journey searches a stock in S1 and votes on it in S2 without starting over.

```swift
@MainActor
func testJourney() throws {
    let session = try JourneySession()
    let app = session.app
    let recorder = ExampleJourney.recorder()
    var code = ""

    try session.scenario("S1") {
        try session.act(as: "A")
        code = ExampleJourney.createAndCopyCode(app, recorder: recorder)
        attachScreenshot(of: app, named: "S1-A-code")
    }

    try session.scenario("S2") {
        let host = try JourneyHandoff.read("hostName")
        try session.act(as: "B")
        ExampleJourney.join(app, code: code, host: host, recorder: recorder)
        attachScreenshot(of: app, named: "S2-B-joined")
    }
}
```

- `try JourneySession()` is the first line. It skips the test unless the runner started it, which keeps live journeys out of the nightly sample run.
- One `try session.scenario("S1") { … }` per scenario of the doc, in the doc's order. `journey.py check` fails when one is missing or out of order. The runner reports each scenario from its own steps. A scenario after a failed one is `SKIP`.
- `try session.act(as: "B")` makes B the signed-in member and returns B's `JourneyAccount`. When B is already signed in, it does nothing, so the app stays where the last step left it. Otherwise it signs out through Profile and signs B in with the `auth/sign-in` steps (`SignInJourney.switchActor`). Start each scenario with `act(as:)` for its first actor, and call it again wherever the doc's Actor column changes.
- A value one actor makes and the next one uses (a cabal name, a handle) is a variable in `testJourney`, set in one scenario's block and read in a later one. It needs no hand-off file.
- A value a setup script makes is read inside the scenario with `try JourneyHandoff.read("cabalID")`. The runner runs `<journey>.setup.sh <scenario>` when the test reaches that scenario, before its block starts.
- Each scenario ends with `attachScreenshot(of:named:)`.
