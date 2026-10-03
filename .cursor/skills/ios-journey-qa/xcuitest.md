# The XCUITest

Two files per flow in `apps/mobile/MonacoUITests/Flows/`. The target compiles any file in that folder. `SignInFlow.swift` and `SignInFlowUITests.swift` are the pattern to copy; `FlowSupport.swift` has the shared pieces named below.

## `<Flow>Flow.swift`: the steps

- An `enum` with `static let id` and `static let version` from the flow doc, one static function per scenario and one **ensure function**: it leaves the app in the flow's end state from any starting state (`SignInFlow.ensureSignedIn`).
- A flow that `requires` another calls that flow's ensure function first. The ensure function is the only thing another flow calls, so each flow's steps exist once.
- Every step in the flow doc is one `recorder.step("S1.2", "…") { … }` carrying the doc's step id. The runner reads timings and the failing step from these.
- Waits poll: `waitForExistence(timeout:)` or `app.waitForFirst(of:timeout:)`, with the seconds the flow doc gives.
- Every assertion message starts with the step id and states what was expected.
- Swift files carry no comments in this repo (`no_comments` in `.swiftlint.yml`): a name, a step label or an assertion message says what a comment would.
- The account comes from `FlowAccount.load()`.

## `<Flow>FlowUITests.swift`: one test per scenario

- Named `test<ScenarioId><Name>`: `testS1SignIn`.
- `try FlowAccount.load()` is the first line. It skips the test unless the runner started it, which keeps live flows out of the nightly sample run.
- The test builds its own starting state through ensure functions, so it passes alone and in any order.
- It ends with `attachScreenshot(of:named:)`.

## Two actors

One `xcodebuild` call drives one simulator, so a two-actor scenario is a sequence of **phases**: `testS1Phase1ACreates`, `testS1Phase2BJoins`, `testS1Phase3ASeesB`. The number is the order and the letter is the actor. The runner runs them in order, each on its actor's simulator, and reports the scenario failed at the first phase that fails.

- Each phase starts with its own actor's ensure functions, so it can be rerun alone.
- A value for the next actor (a cabal id, an invite code) goes through `try FlowHandoff.write("cabalID", value)` and `try FlowHandoff.read("cabalID")`.
- A phase that depends on the other actor's work polls the screen for it, with the flow doc's timeout.
