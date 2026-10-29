import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit
import XCTest

@testable import Monaco

nonisolated final class ProfileShellTests: XCTestCase {
    @MainActor
    func testTheCardStandingAndCashSlotsAreLive() {
        XCTAssertTrue(ProfileCardSlot.isLive)
        XCTAssertTrue(ProfileStandingSlot.isLive)
        XCTAssertTrue(ProfileCashSlot.isLive)
    }

    @MainActor
    func testTheDepositDockIsNotAScrollingSection() {
        let names = ProfileScreen.sections.map { String(describing: $0) }
        XCTAssertEqual(names, ["ProfileCardSlot", "ProfileStandingSlot", "ProfileCashSlot", "ProfileTabsSlot"])
    }

    @MainActor
    func testTheHarnessScenariosForTheTopOfTheProfileResolve() {
        for name in ["ready", "unranked", "noStanding", "hiddenPerformance", "failed"] {
            XCTAssertNotNil(ProfileSampleScenario.matching(["-MonacoProfileSample", name]), name)
        }
    }

    @MainActor
    func testTheSectionsFollowTheDesignOrderWithoutTheV3HeaderAndCounts() {
        let names = ProfileScreen.sections.map { String(describing: $0) }
        XCTAssertEqual(names.prefix(2), ["ProfileCardSlot", "ProfileStandingSlot"])
        XCTAssertFalse(names.contains("ProfileHeaderSlot"))
        XCTAssertFalse(names.contains("ProfileFollowCountsSlot"))
    }

    @MainActor
    func testTheShareRoundNeedsTheProfileAndThePortfolio() {
        let profile = ProfileSampleHarness.sampleProfile(userID: "u", displayName: "Alex", photoURL: nil)
        XCTAssertFalse(ProfileScreen.canShare(profile: nil, portfolioState: .loading))
        XCTAssertFalse(ProfileScreen.canShare(profile: profile, portfolioState: .loading))
        XCTAssertTrue(ProfileScreen.canShare(profile: profile, portfolioState: .loaded(PortfolioSummary(.sample))))
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
            "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."
        )
    }
}

nonisolated final class UserProfileMoreMenuTests: XCTestCase {
    @MainActor
    func testOnlyBlockIsDestructiveAndEveryActionHasAnIcon() {
        XCTAssertEqual(UserProfileMoreMenu.SafetyAction.allCases.map(\.title), ["Report", "Block", "Unblock"])
        for action in UserProfileMoreMenu.SafetyAction.allCases {
            XCTAssertEqual(action.role, action == .block ? .destructive : nil, action.title)
            XCTAssertFalse(action.systemImage.isEmpty, action.title)
        }
    }
}

nonisolated final class MainTabGlyphTests: XCTestCase {
    @MainActor
    func testEveryTabGlyphHasAFilledVariant() {
        for tab in MainTab.allCases {
            XCTAssertNotNil(UIImage(systemName: tab.systemImage), tab.title)
            if tab != .stocks {
                XCTAssertNotNil(UIImage(systemName: tab.systemImage + ".fill"), tab.title)
            }
        }
    }
}
