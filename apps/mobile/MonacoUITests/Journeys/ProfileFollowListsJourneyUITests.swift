import XCTest

nonisolated final class ProfileFollowListsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileFollowListsJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            ProfileFollowListsJourney.followers(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1 followers")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            ProfileFollowListsJourney.following(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2 following")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            ProfileFollowListsJourney.counts(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3 counts")
        }
    }
}
