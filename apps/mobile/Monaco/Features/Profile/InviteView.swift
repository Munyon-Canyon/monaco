import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

nonisolated struct InviteRoute: AppRoute {
    @MainActor func destination() -> some View {
        InviteView()
    }
}

enum InviteCopy {
    static let title = "Invite friends"
    static let body = "Share your link. When a friend joins, you follow each other."
    static let copy = "Copy"
    static let share = "Share"
    static let shareMessage = "Join me on Monaco"
    static let copied = "Link copied."
    static let codeLinkLabel = "This link works too"
    static let deposit = "Deposit"
}

struct InviteView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: InviteLinkModel?

    var body: some View {
        InviteContent(
            state: model?.state ?? .loading,
            handle: environment.viewer?.handle,
            toast: model?.toast,
            retry: { Task { await model?.load() } },
            deposit: { environment.navigator.open(DepositRoute(), in: environment.navigator.selectedTab) },
            dismissToast: { model?.dismissToast() }
        )
        .refreshable { await model?.load() }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
    }

    private func preparedModel() -> InviteLinkModel {
        if let model { return model }
        let created = InviteLinkModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct InviteContent: View {
    let state: InviteLinkState
    let handle: String?
    let toast: String?
    let retry: () -> Void
    let deposit: () -> Void
    let dismissToast: () -> Void

    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                Text(InviteCopy.body)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                content
            }
            .padding(MonacoTheme.Space.gutter)
        }
        .monacoCanvas()
        .navigationTitle(InviteCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            dismissToast()
        }
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .idle, .loading, .pending:
            SkeletonBlock(height: 120, radius: MonacoTheme.Radius.card)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading your invite link")
        case .failed:
            MonacoErrorRow(thing: "your invite link", identifier: "invite-error", retry: retry)
        case .loaded(let links):
            InviteLinkCard(url: links.shareURL) {
                toasts.current = MonacoToast(message: InviteCopy.copied, isSuccess: true)
            }
            if let codeURL = links.codeURL {
                InviteCodeLinkRow(url: codeURL)
            }
            if links.showsUnlockPrompt {
                InviteUnlockPrompt(handle: handle, deposit: deposit)
            }
        }
    }
}

private struct InviteLinkCard: View {
    let url: URL
    let copied: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            InviteLinkText(url: url, font: MonacoTheme.Typo.data, color: MonacoTheme.ink)
                .accessibilityIdentifier("invite-link")
            let actions =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.sm))
                : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
            actions {
                ShareLink(item: url, message: Text(InviteCopy.shareMessage)) {
                    Label(InviteCopy.share, systemImage: "square.and.arrow.up")
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("invite-share")
                Button(action: copy) {
                    Label(InviteCopy.copy, systemImage: "doc.on.doc")
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("invite-copy")
            }
            .monacoFullWidthButtons()
            .padding(.top, MonacoTheme.Space.xs)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            MonacoTheme.surface,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .overlay {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
        }
        .accessibilityElement(children: .contain)
    }

    private func copy() {
        UIPasteboard.general.url = url
        Haptics.selection()
        copied()
    }
}

private struct InviteCodeLinkRow: View {
    let url: URL

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(InviteCopy.codeLinkLabel)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)
            InviteLinkText(url: url, font: MonacoTheme.Typo.dataCaption, color: MonacoTheme.muted)
                .accessibilityIdentifier("invite-code-link")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .accessibilityElement(children: .combine)
    }
}

private struct InviteUnlockPrompt: View {
    let handle: String?
    let deposit: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(InviteLinks.unlockCopy(handle: handle))
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("invite-unlock")
            Button(InviteCopy.deposit, action: deposit)
                .buttonStyle(.monacoSecondary)
                .monacoFullWidthButtons()
                .accessibilityIdentifier("invite-deposit")
        }
        .padding(.top, MonacoTheme.Space.s)
    }
}

private struct InviteLinkText: View {
    let url: URL
    let font: Font
    let color: Color

    var body: some View {
        Text(verbatim: InviteLinks.withoutScheme(url))
            .font(font)
            .foregroundStyle(color)
            .lineLimit(1)
            .minimumScaleFactor(0.4)
            .textSelection(.enabled)
    }
}

#if DEBUG
final class InviteSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-inviteHarness"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "locked"
        return AnyView(NavigationStack { InviteHarnessScreen(unlocked: mode == "unlocked") })
    }
}

private struct InviteHarnessScreen: View {
    let unlocked: Bool

    var body: some View {
        InviteContent(
            state: .loaded(links), handle: "kaicenat", toast: nil, retry: {}, deposit: {}, dismissToast: {})
    }

    private var links: InviteLinks {
        let code = Components.Schemas.MyReferralCode(
            code: "k7m4qx2p", link: "https://monacolabs.xyz/r/k7m4qx2p",
            handleLink: unlocked ? "https://monacolabs.xyz/r/kaicenat" : nil, handleUnlocked: unlocked)
        guard let links = InviteLinks(code) else { preconditionFailure("sample invite link is not a URL") }
        return links
    }
}
#endif
