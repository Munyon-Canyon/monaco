import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct ReferralRoute: AppRoute {
    let code: ReferralCode

    @MainActor func destination() -> some View {
        ReferralDestination(code: code)
    }
}

struct ReferralDestination: View {
    let code: ReferralCode

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var model: ReferralLookupModel?

    var body: some View {
        content
            .task { await load() }
    }

    @ViewBuilder
    private var content: some View {
        switch model?.state {
        case .loaded(.referrer(let referrer)):
            UserProfileScreen(userID: referrer.userID)
        case .failed(let error):
            EmptyState(
                title: ToastCopy.message(for: error), actionTitle: "Try again",
                action: { Task { await load() } }
            )
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
        default:
            VStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 88, height: 88, radius: 44)
                SkeletonBlock(width: 160, height: 20)
                SkeletonBlock(width: 96, height: 14)
            }
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading")
        }
    }

    private func load() async {
        let model = model ?? ReferralLookupModel(api: environment.api, code: code)
        self.model = model
        await model.load()
        if case .loaded(.unknown) = model.state {
            toasts.current = MonacoToast(message: ReferralCopy.invalidCode)
            dismiss()
        }
    }
}
