import XCTest

nonisolated final class ProfileEditJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> XCUIApplication {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        ProfileEditJourney.ensureOnProfile(app, as: account)
        return app
    }

    @MainActor
    func testS1NudgeBanner() throws {
        let app = try start()
        ProfileEditJourney.nudgeBanner(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S1-banner-closed")
    }

    @MainActor
    func testS2Rename() throws {
        let app = try start()
        ProfileEditJourney.renameAndRestore(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S2-renamed")
    }

    @MainActor
    func testS3OpenHandleEditor() throws {
        let app = try start()
        ProfileEditJourney.openHandleEditor(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S3-handle-editor")
    }

    @MainActor
    func testS4ChangeFace() throws {
        let app = try start()
        ProfileEditJourney.changeFace(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S4-face")
    }

    @MainActor
    func testS5NoBannerWhenComplete() throws {
        let app = try start()
        ProfileEditJourney.noBannerWhenComplete(app, recorder: ProfileEditJourney.recorder())
        attachScreenshot(of: app, named: "S5-no-banner")
    }
}
