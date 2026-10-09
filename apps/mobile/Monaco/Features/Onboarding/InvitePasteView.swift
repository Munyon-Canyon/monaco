import MonacoCore
import SwiftUI
import UIKit

enum LaunchRecord {
    static let isFirstLaunch = FirstLaunchMarker(store: UserDefaults.standard).registerLaunch()
}

struct InviteGatedLoginView: View {
    @ObservedObject var auth: PrivyAuthService
    @Environment(ToastCenter.self) private var toasts
    @State private var model = InvitePasteModel(
        store: UserDefaults.standard, now: { Date() }, track: { ReferralAnalytics.capture($0) })
    @State private var step = Step.deciding

    private enum Step {
        case deciding
        case paste
        case login
    }

    var body: some View {
        switch step {
        case .deciding:
            Color.clear.task { await decide() }
        case .paste:
            InvitePasteView(
                onPaste: { url in
                    let result = model.paste(url)
                    toasts.current = MonacoToast(message: result.toast, isSuccess: result == .added)
                    if result == .added { step = .login }
                },
                onSkip: {
                    model.skip()
                    step = .login
                })
        case .login:
            LoginView(auth: auth)
        }
    }

    private func decide() async {
        let offer = await model.shouldOffer(isFirstLaunch: LaunchRecord.isFirstLaunch) {
            let probableURL: PartialKeyPath<UIPasteboard.DetectedValues> = \.probableWebURL
            let found = try? await UIPasteboard.general.detectedPatterns(for: [probableURL])
            return found?.contains(probableURL) ?? false
        }
        step = offer ? .paste : .login
    }
}

struct InvitePasteView: View {
    let onPaste: (URL) -> Void
    let onSkip: () -> Void

    @Environment(\.colorScheme) private var colorScheme

    private var pasteTint: Color {
        colorScheme == .dark ? MonacoTheme.surfaceSunken : MonacoTheme.brandFill
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text("Were you invited?")
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityAddTraits(.isHeader)
                Text("Paste the link your friend sent you.")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .fixedSize(horizontal: false, vertical: true)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                VStack(spacing: MonacoTheme.Space.xs) {
                    PasteButton(payloadType: URL.self) { urls in
                        guard let url = urls.first else { return }
                        Task { @MainActor in onPaste(url) }
                    }
                    .tint(pasteTint)
                    .labelStyle(.titleOnly)
                    .buttonBorderShape(.capsule)
                    .controlSize(.large)
                    .background {
                        if colorScheme == .dark {
                            Capsule().strokeBorder(MonacoTheme.secondaryText, lineWidth: 1).padding(-1)
                        }
                    }
                    .accessibilityLabel("Paste invite")
                    .accessibilityIdentifier("invite-paste-button")

                    Button(action: onSkip) {
                        Text("Skip")
                    }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("invite-skip-button")
                }
                .frame(maxWidth: .infinity)
            }
        }
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("invite-paste-screen")
    }
}
