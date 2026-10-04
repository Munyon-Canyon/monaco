import SwiftUI

enum UserProfileSharedCabalsSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileSharedCabals(userID: context.userID)
    }
}

private struct UserProfileSharedCabals: View {
    let userID: String
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        if userID != environment.viewer?.userID {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Cabals you share")
                Text("Shared cabals show up here soon.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("user-profile-shared-coming")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
