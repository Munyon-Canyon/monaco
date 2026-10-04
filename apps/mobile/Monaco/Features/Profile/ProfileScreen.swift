import SwiftUI

struct ProfileScreen: View {
    static let sections: [any ProfileSection.Type] = [
        ProfileHeaderSlot.self,
        ProfileFollowCountsSlot.self,
        ProfileStatsSlot.self,
        ProfileBalanceSlot.self,
        ProfileCabalsSlot.self,
        ProfileInviteSlot.self,
        ProfileFindFriendsSlot.self,
        ProfileSettingsSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        VStack(spacing: MonacoTheme.Space.gutter) {
            SectionStack(context: (), sections: sections.map { $0.erased })
            Button("Sign out") {
                Task { await environment.signOut() }
            }
            .accessibilityIdentifier("profileSignOutButton")
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
    }
}
