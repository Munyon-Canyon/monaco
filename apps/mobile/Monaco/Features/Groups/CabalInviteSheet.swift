import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated enum CabalInviteMode: Hashable, Sendable {
    case share
    case invited
    case requested
    case unavailable
}

struct CabalInviteSheet: View {
    let cabal: Components.Schemas.Cabal
    var mode: CabalInviteMode = .share
    var link: URL?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var links: InviteLinkModel?
    @State private var access: CabalInviteAccessModel?
    @State private var sharing: String?

    private var shareURL: URL? { link ?? links?.links?.shareURL }

    var body: some View {
        NavigationStack {
            content
                .monacoSheet(
                    title: "Invite to cabal", titleIdentifier: "cabal-invite-title",
                    detents: .votes, showsClose: true
                )
                .toolbar(.hidden, for: .navigationBar)
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .monacoToastCenter(toasts)
        .task {
            guard mode == .share else { return }
            let links = self.links ?? InviteLinkModel(api: environment.api, hints: environment.hints)
            self.links = links
            if link == nil { await links.load() }
        }
        .task {
            guard mode == .share else { return }
            let access = self.access ?? CabalInviteAccessModel(cabalID: cabal.id, api: environment.api)
            self.access = access
            await access.load()
        }
    }

    @ViewBuilder private var content: some View {
        switch mode {
        case .share:
            shareContent
        case .invited:
            outcome(pending: false, title: "You\u{2019}re invited", id: "cabal-invite-preview")
        case .requested:
            outcome(pending: true, title: "Request sent", id: "cabal-invite-pending")
        case .unavailable:
            EmptyState(
                title: "Invitation unavailable", message: "Ask a member to share a new invitation.",
                systemImage: MonacoIcon.link.symbol
            )
            .frame(maxWidth: .infinity)
            .accessibilityIdentifier("cabal-invite-unavailable")
        }
    }

    private func outcome(pending: Bool, title: String, id: String) -> some View {
        OutcomeView(tone: pending ? .pending : .done, title: title, message: cabal.name) {
            Button("View cabal") {
                dismiss()
                environment.navigator.open(
                    CabalRoute(id: cabal.id), in: hostMainTab ?? environment.navigator.selectedTab)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("cabal-invite-view")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(id)
    }

    private var shareContent: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                CabalCover(
                    cabalId: cabal.id, tint: .forGroupId(cabal.id), pictureURL: CabalMark.resolvedURL(cabal.pictureUrl),
                    style: .card
                ) {
                    Text(cabal.name)
                        .font(MonacoTheme.Typo.title)
                        .foregroundStyle(MonacoTheme.onCover)
                        .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 1)
                        .padding(MonacoTheme.Space.gutter)
                }
                .frame(height: CabalInviteMetrics.coverHeight)
                .accessibilityIdentifier("cabal-invite-cover")
                Text("Share a link with your friends.")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .padding(.top, MonacoTheme.Space.m)
                linkBox
                    .padding(.top, MonacoTheme.Space.sm)
                buttons
                    .padding(.top, MonacoTheme.Space.gutter)
                rows
                    .padding(.top, MonacoTheme.Space.m)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollBounceBehavior(.basedOnSize)
    }

    @ViewBuilder private var linkBox: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Group {
                if let shareURL {
                    Text(shareURL.absoluteString)
                        .textSelection(.enabled)
                } else if case .failed? = links?.state {
                    Text("Couldn\u{2019}t load your link.")
                } else {
                    Text(" ").skeletonBar(width: CabalInviteMetrics.skeletonLink)
                }
            }
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.primaryText)
            .padding(MonacoTheme.Space.sm)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
                    .fill(MonacoTheme.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
                    .strokeBorder(MonacoTheme.border, lineWidth: 1)
            )
            .accessibilityIdentifier("cabal-invite-link")
            Text(CabalCopy.cabalLinksOpenSoon)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private var buttons: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            Button("Share link") {
                if let shareURL { SystemShare.present(SharePayload(title: cabal.name, url: shareURL)) }
            }
            .buttonStyle(.monacoPrimary)
            .disabled(shareURL == nil)
            .accessibilityIdentifier("share-card-share")
            Button("Copy link") { copy() }
                .buttonStyle(.monacoOutline)
                .disabled(shareURL == nil)
                .accessibilityIdentifier("share-card-copy")
        }
    }

    private func copy() {
        guard let shareURL else { return }
        UIPasteboard.general.url = shareURL
        Haptics.selection()
        toasts.current = MonacoToast(message: CabalCopy.linkCopied, isSuccess: true)
    }

    private var rows: some View {
        MonacoGroupedList {
            if let standing = access?.standing, standing.canInvite {
                NavigationLink(value: AnyAppRoute(InviteMemberRoute(cabalID: cabal.id, standing: standing))) {
                    MonacoRow(title: "Invite by handle", chevron: true, isLast: false) {
                        SunkenGlyphMark(systemImage: "at")
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("cabal-invite-handle-row")
            }
            Button {
                Task { await ShareCardLauncher.shareCabal(cabal, api: environment.api, toasts: toasts, busy: $sharing) }
            } label: {
                MonacoRow(title: "Share your gain as an image", chevron: true, isLast: true) {
                    SunkenGlyphMark(systemImage: "photo")
                }
            }
            .buttonStyle(.monacoRow)
            .disabled(sharing != nil)
            .accessibilityIdentifier("cabal-invite-gain-row")
        }
        .padding(.horizontal, -MonacoTheme.Space.gutter)
    }
}

enum CabalInviteMetrics {
    static let coverHeight: CGFloat = 140
    static let skeletonLink: CGFloat = 220
}
