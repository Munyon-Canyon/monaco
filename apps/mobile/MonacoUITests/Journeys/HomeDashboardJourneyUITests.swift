import XCTest

nonisolated final class HomeDashboardJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AReadsHome() throws {
        let account = try JourneyAccount.load()
        let cabalID = try JourneyHandoff.read("cabalID")
        let cabalName = try JourneyHandoff.read("cabalName")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        HomeDashboardJourney.readsHome(
            app, cabalID: cabalID, cabalName: cabalName, recorder: HomeDashboardJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-home")
    }

    @MainActor
    func testS2Phase1CEmptySendsToBrowse() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        HomeDashboardJourney.emptySendsToBrowse(app, recorder: HomeDashboardJourney.recorder())
        attachScreenshot(of: app, named: "S2-C-empty")
    }
}
