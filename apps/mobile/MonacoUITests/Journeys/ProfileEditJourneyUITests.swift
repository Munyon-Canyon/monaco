import XCTest

nonisolated final class ProfileEditJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileEditJourney.recorder()
        _ = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            try ProfileEditJourney.changeName(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-renamed")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            ProfileEditJourney.emptyNameRefused(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-name-kept")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            ProfileEditJourney.changePhoto(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-photo")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            try ProfileEditJourney.survivesRelaunch(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4-relaunched")
        }
    }
}
