import XCTest

nonisolated final class ProfileUserProfileJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileUserProfileJourney.recorder()

        try session.scenario("S1") {
            let seed = try ProfileUserProfileJourney.Seed.handedOff()
            try session.act(as: "A")
            ProfileUserProfileJourney.header(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S1 member profile")
        }

        try session.scenario("S2") {
            let seed = try ProfileUserProfileJourney.Seed.handedOff()
            try session.act(as: "A")
            ProfileUserProfileJourney.follow(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S2 after unfollow")
        }

        try session.scenario("S3") {
            let seed = try ProfileUserProfileJourney.Seed.handedOff()
            try session.act(as: "A")
            ProfileUserProfileJourney.sharedCabals(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S3 shared cabals")
        }

        try session.scenario("S4") {
            let seed = try ProfileUserProfileJourney.Seed.handedOff()
            try session.act(as: "A")
            ProfileUserProfileJourney.reportAndBlock(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S4 more menu")
        }

        try session.scenario("S5") {
            let seed = try ProfileUserProfileJourney.Seed.handedOff()
            try session.act(as: "A")
            ProfileUserProfileJourney.ownRow(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S5 own profile")
        }
    }
}
