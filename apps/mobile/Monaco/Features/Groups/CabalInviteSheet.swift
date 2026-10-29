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

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var invite: CabalInviteModel?
    @State private var flip: Double = 0
    @State private var handleRoute: AnyAppRoute?

    private var showsQR: Bool { flip >= 90 }

    private var isAdmin: Bool { CabalInviteCopy.isAdmin(role: cabal.me?.role) }

    var body: some View {
        NavigationStack {
            content
                .toolbar(.hidden, for: .navigationBar)
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
                .navigationDestination(item: $handleRoute) { $0.destination() }
        }
        .monacoToastCenter(toasts)
        .task {
            guard mode == .share else { return }
            let invite = self.invite ?? makeModel()
            self.invite = invite
            await invite.load()
            await invite.observe()
        }
    }

    private func makeModel() -> CabalInviteModel {
        CabalInviteModel(
            api: environment.api, hints: environment.hints, cabalID: cabal.id,
            inviteLinks: InviteLinkModel(api: environment.api, hints: environment.hints))
    }

    @ViewBuilder private var content: some View {
        switch mode {
        case .share:
            shareSheet
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
            .monacoSheet(
                title: "Invite to cabal", titleIdentifier: "cabal-invite-title", detents: .votes, showsClose: true)
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
        .monacoSheet(title: "Invite to cabal", titleIdentifier: "cabal-invite-title", detents: .votes, showsClose: true)
    }

    private var shareSheet: some View {
        VStack(alignment: .leading, spacing: 0) {
            heading
            ScrollView {
                face
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.m)
            }
            .scrollBounceBehavior(.basedOnSize)
            buttons
        }
        .presentationDetents(Set(MonacoSheetDetents.half + (dynamicTypeSize.isAccessibilitySize ? [.large] : [])))
        .presentationDragIndicator(.visible)
        .presentationContentInteraction(.scrolls)
        .presentationBackgroundInteraction(.disabled)
    }

    private var heading: some View {
        HStack(spacing: CabalInviteMetrics.roundGap) {
            Text(invite?.preview?.title ?? CabalInviteCopy.title(name: cabal.name))
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("cabal-invite-title")
            MonacoGlassButton(
                systemImage: showsQR ? "rectangle.on.rectangle" : MonacoIcon.qr.symbol,
                accessibilityLabel: showsQR ? "Show invite card" : "Show QR code", style: .quiet, size: .compact
            ) { toggleFlip() }
            .disabled(invite?.link == nil)
            .accessibilityIdentifier("cabal-invite-qr-toggle")
            if isAdmin { settingsMenu }
            MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet, size: .compact) {
                dismiss()
            }
        }
        .frame(minHeight: 48)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.sm)
    }

    private var settingsMenu: some View {
        Menu {
            Button {
                handleRoute = AnyAppRoute(
                    InviteMemberRoute(cabalID: cabal.id, standing: CabalInviteStanding(role: .creator)))
            } label: {
                Label("Invite by handle", systemImage: "at")
            }
            .accessibilityIdentifier("cabal-invite-handle-row")
            Button {
                dismiss()
                environment.navigator.open(
                    CabalRoute(id: cabal.id, tab: .rules), in: hostMainTab ?? environment.navigator.selectedTab)
            } label: {
                Label(CabalInviteCopy.joiningLine(joinMode: cabal.rules.joinMode), systemImage: "person.badge.plus")
            }
            .accessibilityIdentifier("cabal-invite-joining-row")
        } label: {
            Image(systemName: "gearshape")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(width: CabalInviteMetrics.roundDiameter, height: CabalInviteMetrics.roundDiameter)
                .background(Circle().fill(MonacoTheme.surfaceSunken))
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Circle())
        }
        .frame(minWidth: 44, minHeight: 44)
        .contentShape(Rectangle())
        .accessibilityLabel("Invite settings")
        .accessibilityIdentifier("cabal-invite-settings")
    }

    @ViewBuilder private var face: some View {
        switch invite?.state ?? .idle {
        case .idle, .loading:
            SkeletonBlock(height: CabalInviteMetrics.skeletonCard, radius: MonacoTheme.Radius.card)
                .accessibilityIdentifier("cabal-invite-loading")
        case .failed:
            VStack(spacing: MonacoTheme.Space.m) {
                EmptyState(
                    title: "Couldn\u{2019}t load your invite", message: "Check your connection and try again.",
                    systemImage: MonacoIcon.link.symbol)
                Button("Try again") { Task { await invite?.load() } }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("cabal-invite-retry")
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-invite-failed")
        case .loaded(let preview):
            flipper(preview)
        }
    }

    private func flipper(_ preview: CabalInvitePreview) -> some View {
        CabalInviteFlip(
            angle: flip, front: CabalInviteCard(preview: preview),
            back: backFace(name: preview.name))
    }

    @ViewBuilder private func backFace(name: String) -> some View {
        if let link = invite?.link {
            CabalInviteQR(link: link, cabalName: name)
        }
    }

    private func toggleFlip() {
        let target: Double = showsQR ? 0 : 180
        if reduceMotion {
            flip = target
        } else {
            withAnimation(.easeInOut(duration: CabalInviteMetrics.flipDuration)) { flip = target }
        }
    }

    private var buttons: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Button {
                share()
            } label: {
                Label("Share", systemImage: "square.and.arrow.up")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(invite?.link == nil || invite?.preview == nil)
            .accessibilityIdentifier("share-card-share")
            Button {
                copy()
            } label: {
                Label("Copy link", systemImage: "doc.on.doc")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoOutline)
            .disabled(invite?.link == nil)
            .accessibilityIdentifier("share-card-copy")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
    }

    private func share() {
        guard let link = invite?.link, let preview = invite?.preview else { return }
        SystemShare.present(SharePayload(title: cabal.name, url: link, image: Self.cardImage(preview)))
    }

    private func copy() {
        guard let link = invite?.link else { return }
        UIPasteboard.general.url = link
        Haptics.selection()
        toasts.current = MonacoToast(message: CabalCopy.linkCopied, isSuccess: true)
    }

    @MainActor static func cardImage(_ preview: CabalInvitePreview) -> UIImage? {
        let card = CabalInviteCard(preview: preview)
            .frame(width: CabalInviteMetrics.shareWidth)
            .environment(\.colorScheme, .dark)
            .dynamicTypeSize(.large)
        let renderer = ImageRenderer(content: card)
        renderer.scale = CabalInviteMetrics.shareScale
        return renderer.uiImage
    }
}
