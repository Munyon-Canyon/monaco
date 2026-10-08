import SwiftUI
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
        XCTAssertEqual(SettingsCopy.signOutTitle, "Sign out of Monaco?")
        XCTAssertEqual(
            SettingsCopy.signOutMessage,
            "Your money stays where it is. You'll need a new code to sign back in."
        )
    }
}

nonisolated final class UserProfileMoreMenuTests: XCTestCase {
    @MainActor
    func testReportAndBlockAreDestructiveWithIcons() {
        XCTAssertEqual(UserProfileMoreMenu.SafetyAction.allCases.map(\.title), ["Report", "Block"])
        for action in UserProfileMoreMenu.SafetyAction.allCases {
            XCTAssertEqual(action.role, .destructive, action.title)
            XCTAssertFalse(action.systemImage.isEmpty, action.title)
        }
    }
}
