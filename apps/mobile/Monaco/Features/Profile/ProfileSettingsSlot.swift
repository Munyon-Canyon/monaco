import SwiftUI

enum ProfileSettingsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileSettings()
    }
}

struct ProfileSettings: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Account")
                .padding(.horizontal, MonacoTheme.Space.m)
            MonacoGroupedList {
                NavigationLink {
                    AdvancedSettingsView()
                } label: {
                    MonacoRow(
                        title: "Advanced",
                        subtitle: "Block explorers",
                        chevron: true,
                        isLast: true,
                        leading: { StockMark(systemImage: "link", size: 40) }
                    )
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("profile-advanced-link")
            }
        }
    }
}
