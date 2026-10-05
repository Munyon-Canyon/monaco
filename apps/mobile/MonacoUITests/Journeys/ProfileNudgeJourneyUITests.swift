import XCTest

nonisolated final class ProfileNudgeJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> XCUIApplication {
        let account = try JourneyAccount.load()
        _ = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        ProfileEditJourney.ensureOnProfile(app, as: account)
        return app
    }

    @MainActor
    func testS1PhotoRateLimit() throws {
        let app = try start()
        ProfileNudgeJourney.photoRateLimit(app, recorder: ProfileNudgeJourney.recorder())
        attachScreenshot(of: app, named: "S1-rate-limited")
    }

    @MainActor
    func testS2NudgeBanner() throws {
        let app = try start()
        ProfileNudgeJourney.nudgeBanner(app, recorder: ProfileNudgeJourney.recorder())
        attachScreenshot(of: app, named: "S2-banner-closed")
    }

    @MainActor
    func testS3NoBannerWhenComplete() throws {
        let app = try start()
        ProfileNudgeJourney.noBannerWhenComplete(app, recorder: ProfileNudgeJourney.recorder())
        attachScreenshot(of: app, named: "S3-no-banner")
    }

    @MainActor
    func testS4OpenHandleEditor() throws {
        let app = try start()
        ProfileNudgeJourney.openHandleEditor(app, recorder: ProfileNudgeJourney.recorder())
        attachScreenshot(of: app, named: "S4-handle-editor")
    }
}
