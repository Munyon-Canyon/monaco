import XCTest

nonisolated final class GovernanceWithdrawJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = GovernanceWithdrawJourney.recorder()

        try session.scenario("S1") {
            let cabalName = try JourneyHandoff.read("cabalName")
            let proposalID = try JourneyHandoff.read("proposalID")
            try session.act(as: "A")
            GovernanceWithdrawJourney.proposerWithdraws(
                app, cabalName: cabalName, proposalID: proposalID, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-withdrawn")
            try session.act(as: "B")
            GovernanceWithdrawJourney.voterNoLongerAsked(app, proposalID: proposalID, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-home")
        }
    }
}
