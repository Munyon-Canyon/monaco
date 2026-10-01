import SwiftUI

struct UserProfileScreen: View {
    static let sections: [any UserProfileSection.Type] = [
        UserProfileHeaderSlot.self,
        UserProfileSharedCabalsSlot.self,
    ]

    let context: UserProfileContext
    let sections: [any UserProfileSection.Type]

    init(userID: String, sections: [any UserProfileSection.Type] = Self.sections) {
        self.context = UserProfileContext(userID: userID)
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        if SectionStack<UserProfileContext>.live(sections).isEmpty {
            NotMigratedView(screen: "Profile")
        } else {
            SectionStack(context: context, sections: sections)
        }
    }
}
