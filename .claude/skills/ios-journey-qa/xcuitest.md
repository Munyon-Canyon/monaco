# The XCUITest

Two files per journey in `apps/mobile/MonacoUITests/Journeys/`. The target compiles any file in that folder. `SignInJourney.swift` and `SignInJourneyUITests.swift` are the pattern to copy; `JourneySupport.swift` has the shared pieces named below.

## `<Journey>Journey.swift`: the steps

- An `enum` with `static let id` and `static let version` from the journey doc, and one static function per scenario or per actor's part of a scenario.
- Every step in the journey doc is one `recorder.step("S1.2", "…") { … }` carrying the doc's step id. The runner reads timings and the failing step from these.
- Waits poll: `waitForExistence(timeout:)` or `app.waitForFirst(of:timeout:)`, with the seconds the journey doc gives.
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
