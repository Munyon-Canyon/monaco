import XCTest

nonisolated final class InviteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AAcceptsAndInvites() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        InviteJourney.acceptAndInvite(app, recorder: InviteJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-invited")
    }

    @MainActor
    func testS1Phase2BDeclines() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        InviteJourney.decline(app, recorder: InviteJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-declined")
    }
}
