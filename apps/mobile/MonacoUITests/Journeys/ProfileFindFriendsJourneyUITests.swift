import XCTest

nonisolated final class ProfileFindFriendsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> (XCUIApplication, ProfileFindFriendsJourney.Member) {
        let account = try JourneyAccount.load()
        let member = try ProfileFindFriendsJourney.Member.handedOff()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return (app, member)
    }

    @MainActor
    func testS1BackOut() throws {
        let (app, _) = try start()
        ProfileFindFriendsJourney.backOut(app, recorder: ProfileFindFriendsJourney.recorder())
        attachScreenshot(of: app, named: "S1 back on Profile")
    }

    @MainActor
    func testS2SearchByHandle() throws {
        let (app, member) = try start()
        ProfileFindFriendsJourney.searchByHandle(app, member: member, recorder: ProfileFindFriendsJourney.recorder())
        attachScreenshot(of: app, named: "S2 followed from search")
    }

    @MainActor
    func testS3SearchByName() throws {
        let (app, member) = try start()
        ProfileFindFriendsJourney.searchByName(app, member: member, recorder: ProfileFindFriendsJourney.recorder())
        attachScreenshot(of: app, named: "S3 search by name")
    }
}
