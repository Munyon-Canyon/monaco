import XCTest

nonisolated final class MoneyActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1DepositReceipt() throws {
        let account = try JourneyAccount.load()
        let txn = try JourneyHandoff.read(MoneyActivityJourney.depositKey)
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyActivityJourney.depositReceipt(app, txn: txn, recorder: MoneyActivityJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-activity")
    }

    @MainActor
    func testS2CabalMove() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyActivityJourney.cabalMove(app, run: run, recorder: MoneyActivityJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-cabal")
    }

    @MainActor
    func testS3Withdrawal() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyActivityJourney.withdrawal(app, recorder: MoneyActivityJourney.recorder())
        attachScreenshot(of: app, named: "S3-A-withdrawal")
    }
}
