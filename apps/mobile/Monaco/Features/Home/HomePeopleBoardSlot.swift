import SwiftUI

enum HomePeopleBoardSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Top investors")
            Text("Rankings show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("home-people-board-coming")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }
}
