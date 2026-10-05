import XCTest

nonisolated final class MoneyCashOutJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1ACashesOut() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyCashOutJourney.cashOut(app, run: run, recorder: MoneyCashOutJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-cashed-out")
    }

    @MainActor
    func testS2Phase1AWithdraws() throws {
        let account = try JourneyAccount.load()
        guard let address = ProcessInfo.processInfo.environment["MONACO_QA_REFUND_ADDRESS"], !address.isEmpty else {
            XCTFail("no {QA.refund_address}: scripts/qa/journey.py passes MONACO_QA_REFUND_ADDRESS")
            return
        }
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyCashOutJourney.withdraw(app, refundAddress: address, recorder: MoneyCashOutJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-withdrawing")
    }
}
