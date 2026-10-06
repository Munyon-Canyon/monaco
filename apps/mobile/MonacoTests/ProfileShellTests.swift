import XCTest

@testable import Monaco

nonisolated final class ProfileShellTests: XCTestCase {
    @MainActor
    func testFollowCountsAndFindFriendsSlotsAreLive() {
        XCTAssertTrue(ProfileFollowCountsSlot.isLive)
        XCTAssertTrue(ProfileFindFriendsSlot.isLive)
    }

    @MainActor
    func testFollowListRouteOpensTheListItNames() throws {
        let destination: Any = FollowListRoute(userID: "u", kind: .followers).destination()
        let list = try XCTUnwrap(destination as? FollowListView)
        XCTAssertEqual(list.userID, "u")
        XCTAssertEqual(list.kind, .followers)
    }

    @MainActor
    func testFriendsRouteOpensTheFriendsScreen() {
        let friends: Any = FriendsRoute().destination()
        XCTAssertTrue(friends is FriendsScreen)
    }

    @MainActor
    func testSignOutDialogCopy() {
        XCTAssertEqual(ProfileScreen.signOutTitle, "Sign out of Monaco?")
        XCTAssertEqual(
            ProfileScreen.signOutMessage,
            "Your money stays where it is. You'll need a new code to sign back in."
        )
    }
}
