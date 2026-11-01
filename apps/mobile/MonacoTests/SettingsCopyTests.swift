import Foundation
import Testing

@testable import Monaco

@MainActor
struct SettingsCopyTests {
    @Test func theRouteOpensTheSettingsScreen() {
        #expect(SettingsRoute().destination() is SettingsView)
    }

    @Test func theRowsReadInScreenOrder() {
        #expect(
            SettingsRow.allCases.map(\.title) == [
                "Notifications", "Edit profile", "Find friends", "Invite friends", "View as others",
                "Account activity", "Withdraw", "Blocked people", "Block explorers", "Contact support", "Sign out",
                "Delete account",
            ])
    }

    @Test func theProfileLinksLiveInSettingsUnderTheirOldIdentifiers() {
        let moved: [SettingsRow] = [.findFriends, .inviteFriends, .viewAsOthers]
        #expect(Array(SettingsRow.accountRows.prefix(4)) == [.editProfile] + moved)
        #expect(moved.map(\.identifier) == ["profile-find-friends", "profile-invite-row", "profile-view-as-others"])
        #expect(
            moved.map(\.subtitle) == [
                "Search people and contacts", "Share your invite link", "See your profile as others do",
            ])
        #expect(moved.map(\.systemImage) == ["person.2", "square.and.arrow.up", "eye"])
    }

    @Test func contactSupportOpensAMailLinkToTheSupportAddress() {
        #expect(SettingsRow.accountRows.contains(.contactSupport))
        #expect(SettingsRow.contactSupport.subtitle == "support@monacolabs.xyz")
        #expect(SettingsRow.contactSupport.identifier == "settings-contact-support")
        #expect(SettingsCopy.supportURL.absoluteString == "mailto:support@monacolabs.xyz?subject=Monaco%20support")
    }

    @Test func theAccountGroupEndsWithSignOutAndDeleteStandsApart() {
        #expect(SettingsRow.accountRows.last == .signOut)
        #expect(!SettingsRow.accountRows.contains(.deleteAccount))
    }

    @Test func theSignOutPanelCopy() {
        #expect(SettingsCopy.signOutTitle == "Sign out of Monaco?")
        #expect(
            SettingsCopy.signOutMessage
                == "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."
        )
    }

    @Test func theBlockedPeopleRowOpensItsScreen() {
        #expect(BlockedPeopleRoute().destination() is BlockedPeopleView)
    }

    @Test func theFooterNamesTheReleaseAndTheBuild() {
        let info: [String: Any] = ["CFBundleShortVersionString": "1.4", "CFBundleVersion": "212"]
        #expect(SettingsCopy.version(info: info) == "Monaco 1.4 (212)")
    }
}
