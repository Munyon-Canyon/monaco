import MonacoCore
import MonacoReferrals
import SwiftUI

struct JoinLinkSheet: View {
    static let refusal = "That isn't a cabal invite link."

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        InvitePasteView(onPaste: open, onSkip: { dismiss() })
            .presentationDetents([.medium])
            .monacoToastCenter(toasts)
            .accessibilityIdentifier("join-link-sheet")
    }

    private func open(_ url: URL) {
        guard ReferralLink.parseTarget(url) != nil else {
            toasts.current = MonacoToast(message: Self.refusal)
            return
        }
        dismiss()
        let navigator = environment.navigator
        Task { await DeepLinkRouter.handle(url, navigator: navigator) }
    }
}
