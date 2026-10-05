import XCTest

nonisolated final class SettingsDeleteAccountJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func devToken() throws -> String {
        _ = try JourneyAccount.load()
        return try JourneyHandoff.read("devToken")
    }

    @MainActor
    func testS1ReadAndBackOut() throws {
        let token = try devToken()
        let app = XCUIApplication.monacoForJourneys()
        SettingsDeleteAccountJourney.readAndBackOut(
            app, devToken: token, recorder: SettingsDeleteAccountJourney.recorder())
        attachScreenshot(of: app, named: "S1 checklist after Cancel")
    }

    @MainActor
    func testS2Delete() throws {
        let token = try devToken()
        let app = XCUIApplication.monacoForJourneys()
        SettingsDeleteAccountJourney.delete(app, devToken: token, recorder: SettingsDeleteAccountJourney.recorder())
        attachScreenshot(of: app, named: "S2 deleted")
    }

    @MainActor
    func testS3CabalOnChecklist() throws {
        let token = try devToken()
        let cabalID = try JourneyHandoff.read("cabalID")
        let cabalName = try JourneyHandoff.read("cabalName")
        let app = XCUIApplication.monacoForJourneys()
        SettingsDeleteAccountJourney.cabalOnChecklist(
            app, devToken: token, cabalID: cabalID, cabalName: cabalName,
            recorder: SettingsDeleteAccountJourney.recorder())
        attachScreenshot(of: app, named: "S3 cabal on the checklist")
    }
}
