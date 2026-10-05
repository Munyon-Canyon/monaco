import XCTest

nonisolated final class CabalsPauseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1APausedShowsWhy() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsPauseJourney.pausedShowsWhy(app, run: run, recorder: CabalsPauseJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-paused")
    }

    @MainActor
    func testS2Phase1ARunningShowsNothing() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsPauseJourney.runningShowsNothing(app, run: run, recorder: CabalsPauseJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-running")
    }
}
