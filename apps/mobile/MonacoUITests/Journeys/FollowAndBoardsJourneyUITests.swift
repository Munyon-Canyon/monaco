import XCTest

nonisolated final class FollowAndBoardsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = FollowAndBoardsJourney.recorder()

        try session.scenario("S1") {
            let seed = try FollowAndBoardsJourney.Seed.handedOff()
            try session.act(as: "A")
            FollowAndBoardsJourney.bothRank(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S1 Top investors")
        }

        try session.scenario("S2") {
            let seed = try FollowAndBoardsJourney.Seed.handedOff()
            try session.act(as: "A")
            FollowAndBoardsJourney.friendsWithNoFollows(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S2 Everyone again")
        }

        try session.scenario("S3") {
            let seed = try FollowAndBoardsJourney.Seed.handedOff()
            try session.act(as: "A")
            FollowAndBoardsJourney.followFromBoard(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S3 Friends after the follow")
        }

        try session.scenario("S4") {
            let seed = try FollowAndBoardsJourney.Seed.handedOff()
            try session.act(as: "B")
            FollowAndBoardsJourney.followerSeesIt(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S4 follower profile")
        }

        try session.scenario("S5") {
            let seed = try FollowAndBoardsJourney.Seed.handedOff()
            try session.act(as: "A")
            FollowAndBoardsJourney.unfollow(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S5 Friends after the unfollow")
        }
    }
}
