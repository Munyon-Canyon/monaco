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
            let seed = try RankingLeaderboardsJourney.Seed.handedOff()
            try session.act(as: "A")
            RankingLeaderboardsJourney.home(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-leaderboard")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            RankingLeaderboardsJourney.cabalsTab(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-cabals")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            RankingLeaderboardsJourney.friends(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-friends")
        }
    }
}
