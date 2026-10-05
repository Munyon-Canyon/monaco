import XCTest

nonisolated final class MoneyWithdrawJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AWithdrawsAll() throws {
        let account = try JourneyAccount.load()
        let address = try MoneyWithdrawJourney.refundAddress()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyWithdrawJourney.withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-withdrawn")
    }
}
