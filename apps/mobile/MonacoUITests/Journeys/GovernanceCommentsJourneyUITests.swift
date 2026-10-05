import XCTest

nonisolated final class GovernanceCommentsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AReadAndPost() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceCommentsJourney.readAndPost(app, run: run, recorder: GovernanceCommentsJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-posted")
    }

    @MainActor
    func testS2Phase1AReply() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceCommentsJourney.reply(app, run: run, recorder: GovernanceCommentsJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-replied")
    }
}
