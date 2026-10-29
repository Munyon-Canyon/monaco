import MonacoReferrals
import SwiftUI

enum JoinLinkCopy {
    static let refusal = "That isn't a cabal invite link."

    static func accepts(_ url: URL) -> Bool {
        ReferralLink.parseTarget(url) != nil
    }
}

struct JoinLinkSheet: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        InvitePasteView(
            onPaste: { url in
                guard JoinLinkCopy.accepts(url) else {
                    toasts.current = MonacoToast(message: JoinLinkCopy.refusal)
                    return
                }
                dismiss()
                Task { await DeepLinkRouter.handle(url, navigator: environment.navigator) }
            },
            onSkip: { dismiss() }
        )
        .presentationDetents(Set(MonacoSheetDetents.half))
        .accessibilityIdentifier("join-link-sheet")
    }
}
