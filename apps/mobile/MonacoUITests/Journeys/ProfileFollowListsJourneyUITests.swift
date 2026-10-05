import XCTest

nonisolated final class ProfileFollowListsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> XCUIApplication {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return app
    }

    @MainActor
    func testS1Followers() throws {
        let app = try start()
        ProfileFollowListsJourney.followers(app, recorder: ProfileFollowListsJourney.recorder())
        attachScreenshot(of: app, named: "S1 followers")
    }

    @MainActor
    func testS2Following() throws {
        let app = try start()
        ProfileFollowListsJourney.following(app, recorder: ProfileFollowListsJourney.recorder())
        attachScreenshot(of: app, named: "S2 following")
    }

    @MainActor
    func testS3Counts() throws {
        let app = try start()
        ProfileFollowListsJourney.counts(app, recorder: ProfileFollowListsJourney.recorder())
        attachScreenshot(of: app, named: "S3 counts")
    }
}
