import XCTest

nonisolated final class ProfileEditJourneyUITests: XCTestCase {
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
    func testS1ChangeName() throws {
        let app = try start()
        try ProfileEditJourney.changeName(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S1-renamed")
    }

    @MainActor
    func testS2EmptyNameRefused() throws {
        let app = try start()
        ProfileEditJourney.emptyNameRefused(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S2-name-kept")
    }

    @MainActor
    func testS3ChangePhoto() throws {
        let app = try start()
        ProfileEditJourney.changePhoto(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S3-photo")
    }

    @MainActor
    func testS4SurvivesRelaunch() throws {
        let app = try start()
        try ProfileEditJourney.survivesRelaunch(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S4-relaunched")
    }

    @MainActor
    func testS5PhotoRateLimit() throws {
        let app = try start()
        ProfileEditJourney.photoRateLimit(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S5-rate-limited")
    }

    @MainActor
    func testS6NudgeBanner() throws {
        let app = try start()
        ProfileEditJourney.nudgeBanner(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S6-banner-closed")
    }

    @MainActor
    func testS7NoBannerWhenComplete() throws {
        let app = try start()
        ProfileEditJourney.noBannerWhenComplete(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S7-no-banner")
    }

    @MainActor
    func testS8OpenHandleEditor() throws {
        let app = try start()
        ProfileEditJourney.openHandleEditor(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S8-handle-editor")
    }
}
