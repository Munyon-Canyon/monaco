import XCTest

nonisolated final class ProfileNudgeJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileNudgeJourney.recorder()
        _ = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            ProfileNudgeJourney.photoRateLimit(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-rate-limited")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            ProfileNudgeJourney.nudgeBanner(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-banner-closed")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            ProfileNudgeJourney.noBannerWhenComplete(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-no-banner")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            ProfileNudgeJourney.openHandleEditor(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4-handle-editor")
        }
    }
}
