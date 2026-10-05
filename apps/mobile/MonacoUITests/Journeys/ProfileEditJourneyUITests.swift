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
}
