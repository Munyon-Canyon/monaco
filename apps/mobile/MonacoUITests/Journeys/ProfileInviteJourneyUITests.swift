import XCTest

nonisolated final class ProfileInviteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileInviteJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            ProfileInviteJourney.copyLink(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-copied")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            ProfileInviteJourney.shareLink(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-shared")
        }
    }
}
