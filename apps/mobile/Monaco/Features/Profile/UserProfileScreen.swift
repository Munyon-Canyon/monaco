import MonacoCore
import SwiftUI

struct UserProfileScreen: View {
    static let sections: [any UserProfileSection.Type] = [
        UserProfileHeaderSlot.self,
        UserProfileSharedCabalsSlot.self,
    ]

    let context: UserProfileContext
    let sections: [any UserProfileSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @State private var refresh = ScreenRefresh()
    @State private var model: UserProfileModel?

    init(userID: String, sections: [any UserProfileSection.Type] = Self.sections) {
        self.context = UserProfileContext(userID: userID)
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        Group {
            if SectionStack<UserProfileContext>.live(sections).isEmpty {
                NotMigratedView(screen: "Profile")
            } else {
                SectionStack(context: context, sections: sections)
            }
        }
        .environment(refresh)
        .environment(model)
        .task(id: context.userID) {
            let model = prepared()
            refresh.register("user-profile-header") { await model.refresh() }
            if model.profile == nil {
                await model.load()
            } else {
                await model.refresh()
            }
        }
        .refreshable { await refresh.run() }
    }

    private func prepared() -> UserProfileModel {
        if let model, model.userID == context.userID { return model }
        let created = UserProfileModel(userID: context.userID, api: environment.api)
        model = created
        return created
    }
}
