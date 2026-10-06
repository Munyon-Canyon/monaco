import XCTest

nonisolated final class AgentsTradingBotJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = AgentsTradingBotJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            AgentsTradingBotJourney.openBot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-bot")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            AgentsTradingBotJourney.proposeBot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-proposed")
        }
    }
}
