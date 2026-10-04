import XCTest

nonisolated final class GovernanceWithdrawJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AWithdraws() throws {
        let account = try JourneyAccount.load()
        let cabalName = try JourneyHandoff.read("cabalName")
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceWithdrawJourney.proposerWithdraws(
            app, cabalName: cabalName, proposalID: proposalID, recorder: GovernanceWithdrawJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-withdrawn")
    }

    @MainActor
    func testS1Phase2BNoLongerAsked() throws {
        let account = try JourneyAccount.load()
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceWithdrawJourney.voterNoLongerAsked(
            app, proposalID: proposalID, recorder: GovernanceWithdrawJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-home")
    }
}
