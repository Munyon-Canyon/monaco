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
                "Notifications", "Activity", "Withdraw", "Advanced", "Delete account",
            ])
        #expect(SettingsRow.advanced.subtitle == "Block explorers")
    }

    @Test func theFooterNamesTheReleaseAndTheBuild() {
        let info: [String: Any] = ["CFBundleShortVersionString": "1.4", "CFBundleVersion": "212"]
        #expect(SettingsCopy.version(info: info) == "Monaco 1.4 (212)")
    }
}
