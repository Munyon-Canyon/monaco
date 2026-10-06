import XCTest

nonisolated final class ProfileFindFriendsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileFindFriendsJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            ProfileFindFriendsJourney.backOut(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1 back on Profile")
        }

        try session.scenario("S2") {
            let member = try ProfileFindFriendsJourney.Member.handedOff()
            try session.act(as: "A")
            ProfileFindFriendsJourney.searchByHandle(app, member: member, recorder: recorder)
            attachScreenshot(of: app, named: "S2 followed from search")
        }

        try session.scenario("S3") {
            let member = try ProfileFindFriendsJourney.Member.handedOff()
            try session.act(as: "A")
            ProfileFindFriendsJourney.searchByName(app, member: member, recorder: recorder)
            attachScreenshot(of: app, named: "S3 search by name")
        }
    }
}
