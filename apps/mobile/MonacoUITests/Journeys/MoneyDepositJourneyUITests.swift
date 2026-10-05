import XCTest

nonisolated final class MoneyDepositJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1CopyAddress() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyDepositJourney.copyAddress(app, recorder: MoneyDepositJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-how-it-works")
    }

    @MainActor
    func testS2ChooseMethod() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        MoneyDepositJourney.chooseMethod(app, recorder: MoneyDepositJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-chooser")
    }
}
