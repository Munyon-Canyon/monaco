import XCTest

nonisolated final class CabalsPotJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1APotAndSlice() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsPotJourney.potAndSlice(app, run: run, recorder: CabalsPotJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-pot")
    }

    @MainActor
    func testS2Phase1AHoldings() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsPotJourney.holdings(app, run: run, recorder: CabalsPotJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-holdings")
    }
}
