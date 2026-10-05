import XCTest

nonisolated final class ProfileInviteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1ACopyLink() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        ProfileInviteJourney.copyLink(app, recorder: ProfileInviteJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-copied")
    }

    @MainActor
    func testS2Phase1AShareLink() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        ProfileInviteJourney.shareLink(app, recorder: ProfileInviteJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-shared")
    }
}
