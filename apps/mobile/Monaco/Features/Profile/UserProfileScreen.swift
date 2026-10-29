import MonacoAPI
import MonacoCore
import SwiftUI

struct UserProfileScreen: View {
    static let sections: [any UserProfileSection.Type] = [
        UserProfileHeaderSlot.self,
        UserProfileTabsSlot.self,
    ]

    let context: UserProfileContext
    let sections: [any UserProfileSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
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
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
        .environment(refresh)
        .environment(model)
        .toolbar {
            if model?.phase == .loaded {
                ToolbarItem(placement: .topBarTrailing) {
                    MonacoGlassButton(
                        icon: .share, accessibilityLabel: "Share profile", style: .quiet, placement: .toolbar
                    ) {
                        share()
                    }
                    .accessibilityIdentifier("user-profile-share")
                }
                if context.userID != environment.viewer?.userID {
                    ToolbarItem(placement: .topBarTrailing) { UserProfileMoreMenu().environment(model) }
                }
            }
        }
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

    private func share() {
        guard let model else { return }
        Task { @MainActor in
            do {
                let code = try await environment.api.read { try await $0.getMyReferralCode().ok.body.json }
                guard let links = InviteLinks(code) else { return }
                SystemShare.present(
                    SharePayload(
                        title: "\(model.displayName) (@\(model.handle)) on Monaco", url: links.shareURL))
            } catch {
                toasts.show(APIError(error))
            }
        }
    }

    private func prepared() -> UserProfileModel {
        if let model, model.userID == context.userID { return model }
        let created = UserProfileModel(userID: context.userID, api: environment.api)
        model = created
        return created
    }
}

extension UserProfileScreen {
    init(userID: String, preview: UserPreview?) {
        self.context = UserProfileContext(userID: userID, preview: preview)
        self.sections = Self.sections
    }
}
