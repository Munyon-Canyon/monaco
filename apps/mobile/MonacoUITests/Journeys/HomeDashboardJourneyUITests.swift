import XCTest

nonisolated final class HomeDashboardJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = HomeDashboardJourney.recorder()

        try session.scenario("S1") {
            let cabalID = try JourneyHandoff.read("cabalID")
            let cabalName = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            HomeDashboardJourney.readsHome(app, cabalID: cabalID, cabalName: cabalName, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-home")
        }

        try session.scenario("S2") {
            try session.act(as: "C")
            HomeDashboardJourney.emptySendsToBrowse(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-C-empty")
        }
    }
}
