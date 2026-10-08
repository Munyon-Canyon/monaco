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

    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        SectionStack(context: (), sections: sections.map { $0.erased })
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
    }
}
