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
    @State private var cabalsModel: UserCabalsModel?

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
        .environment(cabalsModel)
        .environment(model)
        .toolbar {
            if model?.phase == .loaded {
                if context.userID != environment.viewer?.userID {
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        shareButton
                        UserProfileMoreMenu().environment(model)
                    }
                } else {
                    ToolbarItem(placement: .topBarTrailing) { shareButton }
                }
            }
        }
        .task(id: context.userID) {
            let model = prepared()
            let cabals = preparedCabals()
            refresh.register("user-profile-header") { await model.refresh() }
            refresh.register("user-profile-cabals") { await cabals.load() }
            async let loadedCabals: Void = cabals.load()
            if model.profile == nil {
                await model.load()
            } else {
                await model.refresh()
            }
            await loadedCabals
        }
        .refreshable { await refresh.run() }
    }

    private var shareButton: some View {
        MonacoGlassButton(
            icon: .share, accessibilityLabel: "Share profile", style: .quiet, placement: .toolbar
        ) {
            share()
        }
        .accessibilityIdentifier("user-profile-share")
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

    private func preparedCabals() -> UserCabalsModel {
        if let cabalsModel, cabalsModel.userID == context.userID { return cabalsModel }
        let created = UserCabalsModel(
            userID: context.userID, viewerID: environment.viewer?.userID, api: environment.api)
        cabalsModel = created
        return created
    }
}

extension UserProfileScreen {
    init(userID: String, preview: UserPreview?) {
        self.context = UserProfileContext(userID: userID, preview: preview)
        self.sections = Self.sections
    }
}
