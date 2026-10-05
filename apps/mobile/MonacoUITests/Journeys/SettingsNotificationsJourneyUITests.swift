import XCTest

nonisolated final class SettingsNotificationsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> XCUIApplication {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return app
    }

    @MainActor
    func testS1OffBeforeAsking() throws {
        let app = try start()
        SettingsNotificationsJourney.offBeforeAsking(app, recorder: SettingsNotificationsJourney.recorder())
        attachScreenshot(of: app, named: "S1 Notifications Off")
    }

    @MainActor
    func testS2PrePrompt() throws {
        let cabalName = try JourneyHandoff.read("cabalName")
        let app = try start()
        SettingsNotificationsJourney.prePrompt(
            app, cabalName: cabalName, recorder: SettingsNotificationsJourney.recorder())
        attachScreenshot(of: app, named: "S2 after Allow")
    }

    @MainActor
    func testS3OnAndOpensSettings() throws {
        let app = try start()
        SettingsNotificationsJourney.onAndOpensSettings(app, recorder: SettingsNotificationsJourney.recorder())
        attachScreenshot(of: app, named: "S3 back from iOS Settings")
    }
}
