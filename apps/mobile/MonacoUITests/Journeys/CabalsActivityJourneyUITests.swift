import XCTest

nonisolated final class CabalsActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AReadsActivity() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try CabalsActivityJourney.readAndOpenReceipt(app, run: run, recorder: CabalsActivityJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-receipt")
    }

    @MainActor
    func testS2Phase1ARetries() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try CabalsActivityJourney.retryFailedTrade(app, run: run, recorder: CabalsActivityJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-failed-receipt")
    }
}
