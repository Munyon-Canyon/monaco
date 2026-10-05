import XCTest

nonisolated final class CabalsBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1BrowsesAndJoins() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsBrowseJourney.browses(app, run: run, recorder: CabalsBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-requested")
    }

    @MainActor
    func testS2SeesTheReturnChart() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsBrowseJourney.seesReturnChart(app, recorder: CabalsBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-return-chart")
    }

    @MainActor
    func testS3SeesTopCabals() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsBrowseJourney.seesTopCabals(app, recorder: CabalsBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S3-A-top-cabals")
    }
}
