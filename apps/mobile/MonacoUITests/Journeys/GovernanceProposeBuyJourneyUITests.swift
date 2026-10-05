import XCTest

nonisolated final class GovernanceProposeBuyJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AProposesBuy() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeBuyJourney.proposeBuy(app, run: run, recorder: GovernanceProposeBuyJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-proposal-sent")
    }
}
