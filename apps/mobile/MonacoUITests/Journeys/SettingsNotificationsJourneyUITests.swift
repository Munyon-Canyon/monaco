import XCTest

nonisolated final class SettingsNotificationsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = SettingsNotificationsJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "B")
            SettingsNotificationsJourney.offBeforeAsking(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1 Notifications Off")
        }

        try session.scenario("S2") {
            let cabalName = try JourneyHandoff.read("cabalName")
            try session.act(as: "B")
            SettingsNotificationsJourney.prePrompt(app, cabalName: cabalName, recorder: recorder)
            attachScreenshot(of: app, named: "S2 after Allow")
        }

        try session.scenario("S3") {
            try session.act(as: "B")
            SettingsNotificationsJourney.onAndOpensSettings(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3 back from iOS Settings")
        }
    }
}
