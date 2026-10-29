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
            CabalsEditRulesJourney.creatorReadsRules(app, run: run, recorder: recorder)
            CabalsEditRulesJourney.creatorRenames(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-renamed")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsEditRulesJourney.creatorProposesSides(app, run: run, recorder: recorder)
            try session.act(as: "B")
            CabalsEditRulesJourney.memberVotes(app, run: run, recorder: recorder)
            try session.act(as: "A")
            CabalsEditRulesJourney.creatorSeesSides(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-sides-on")
        }
    }
}
