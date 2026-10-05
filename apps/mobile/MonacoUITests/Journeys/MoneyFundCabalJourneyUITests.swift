import XCTest

nonisolated final class MoneyFundCabalJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1PickAmount() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyFundCabalJourney.pickAmount(app, run: run, recorder: MoneyFundCabalJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-amount")
    }

    @MainActor
    func testS2AddToPot() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyFundCabalJourney.addToPot(app, run: run, recorder: MoneyFundCabalJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-funded")
    }
}
