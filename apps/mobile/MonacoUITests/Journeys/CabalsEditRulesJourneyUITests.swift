import XCTest

nonisolated final class CabalsEditRulesJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsEditRulesJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            let member = try session.account("B")
            CabalsEditRulesJourney.creatorEdits(app, run: run, recorder: recorder)
            try CabalsEditRulesJourney.creatorPicksVoters(app, run: run, member: member.name, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-edited")
        }
    }
}
