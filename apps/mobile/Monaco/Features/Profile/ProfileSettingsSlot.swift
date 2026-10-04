import SwiftUI

enum ProfileSettingsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileSettingsRow()
    }
}

private struct ProfileSettingsRow: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        MonacoGroupedList {
            Button {
                environment.navigator.open(SettingsRoute(), in: .profile)
            } label: {
                MonacoRow(
                    title: "Settings",
                    chevron: true,
                    isLast: true,
                    leading: { StockMark(systemImage: "gearshape", size: 40) }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-settings-row")
        }
    }
}
