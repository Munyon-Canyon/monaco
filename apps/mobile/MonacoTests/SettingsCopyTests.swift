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
                "Notifications", "Edit profile", "Account activity", "Withdraw", "Blocked people", "Block explorers",
                "Sign out", "Delete account",
            ])
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
