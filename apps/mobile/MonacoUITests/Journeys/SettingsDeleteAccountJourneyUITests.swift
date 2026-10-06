import XCTest

nonisolated final class SettingsDeleteAccountJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = SettingsDeleteAccountJourney.recorder()

        try session.scenario("S1") {
            let token = try JourneyHandoff.read("devToken")
            SettingsDeleteAccountJourney.readAndBackOut(app, devToken: token, recorder: recorder)
            attachScreenshot(of: app, named: "S1 checklist after Cancel")
        }

        try session.scenario("S2") {
            let token = try JourneyHandoff.read("devToken")
            SettingsDeleteAccountJourney.delete(app, devToken: token, recorder: recorder)
            attachScreenshot(of: app, named: "S2 deleted")
        }

        try session.scenario("S3") {
            let token = try JourneyHandoff.read("devToken")
            let cabalID = try JourneyHandoff.read("cabalID")
            let cabalName = try JourneyHandoff.read("cabalName")
            SettingsDeleteAccountJourney.emptyCabalOffChecklist(
                app, devToken: token, cabalID: cabalID, cabalName: cabalName, recorder: recorder)
            attachScreenshot(of: app, named: "S3 empty cabal off the checklist")
        }
    }
}
