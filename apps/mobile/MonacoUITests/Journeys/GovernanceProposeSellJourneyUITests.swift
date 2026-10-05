import XCTest

nonisolated final class GovernanceProposeSellJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AProposesSell() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeSellJourney.proposeSell(app, run: run, recorder: GovernanceProposeSellJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-proposal-sent")
    }
}
