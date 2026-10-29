import XCTest

nonisolated final class HomeActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = HomeActivityJourney.recorder()

        try session.scenario("S1") {
            let waitingKey = try JourneyHandoff.read("waitingKey")
            let replyKey = try JourneyHandoff.read("replyKey")
            let resultKey = try JourneyHandoff.read("resultKey")
            try session.act(as: "A")
            let count = HomeActivityJourney.opensFromBell(
                app, waitingKey: waitingKey, replyKey: replyKey, resultKey: resultKey, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-catch-up")
            HomeActivityJourney.clearsAResult(app, resultKey: resultKey, count: count, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-cleared")
            HomeActivityJourney.votesFromARow(app, waitingKey: waitingKey, count: count, recorder: recorder)
            HomeActivityJourney.badgeDrops(app, count: count, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-home")
        }
    }
}
