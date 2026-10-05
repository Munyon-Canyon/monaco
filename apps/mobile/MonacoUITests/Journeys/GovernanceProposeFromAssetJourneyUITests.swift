import XCTest

nonisolated final class GovernanceProposeFromAssetJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AProposesFromAsset() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeFromAssetJourney.proposeFromAsset(
            app, run: run, recorder: GovernanceProposeFromAssetJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-proposal-sent")
    }
}
