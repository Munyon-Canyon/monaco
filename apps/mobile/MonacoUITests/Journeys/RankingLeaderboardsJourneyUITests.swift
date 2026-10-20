import XCTest

nonisolated final class RankingLeaderboardsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = RankingLeaderboardsJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            RankingLeaderboardsJourney.homeLeaderboard(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-home-leaderboard")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            RankingLeaderboardsJourney.topCabals(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-top-cabals")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            RankingLeaderboardsJourney.viewAll(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-leaderboard")
        }
    }
}
