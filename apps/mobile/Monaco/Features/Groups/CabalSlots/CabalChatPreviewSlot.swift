import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalChatPreviewSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalChatPreviewLive(cabalID: context.cabalID)
    }
}

private struct CabalChatPreviewLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var cabalModel
    @State private var model: CabalChatPreviewModel?

    private var isMember: Bool {
        guard let cabal = cabalModel?.cabal else { return false }
        return cabal.me != nil && !CabalClosedModel.isReadOnly(cabal.status)
    }

    var body: some View {
        Group {
            if isMember {
                CabalChatPreview(model: model, cabalName: cabalModel?.cabal?.name ?? "") { route in
                    environment.navigator.open(route, in: environment.navigator.selectedTab)
                }
            } else {
                Color.clear.frame(height: 0)
            }
        }
        .task(id: retry.tick * 2 + (isMember ? 1 : 0)) {
            guard isMember else { return }
            let model =
                self.model
                ?? CabalChatPreviewModel(
                    cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.model = model
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }
}

struct CabalChatPreview: View {
    let model: CabalChatPreviewModel?
    let cabalName: String
    let open: (any AppRoute) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Chat", trailing: "Open chat", actionIdentifier: "cabal-chat-open") { openChat() }
            card
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-chat")
    }

    private func openChat() {
        guard let model else { return }
        open(ChatRoute(cabalID: model.cabalID))
    }

    @ViewBuilder private var card: some View {
        switch model?.phase ?? .loading {
        case .loading:
            MonacoCard(style: .tight) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(width: 34, height: 34, radius: 17)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonBlock(width: 120, height: 14)
                        SkeletonBlock(height: 14)
                    }
                }
            }
            .accessibilityHidden(true)
        case .failed:
            Button(action: openChat) {
                MonacoRow(title: "Open chat", chevron: true, isLast: true) {
                    SunkenGlyphMark(icon: .chat)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("cabal-action-chat")
        case .empty:
            Button(action: openChat) {
                MonacoCard(style: .tight) {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        Text(CabalCopy.noMessagesTitle)
                            .font(MonacoTheme.Typo.subheadStrong)
                            .foregroundStyle(MonacoTheme.primaryText)
                        Text(CabalCopy.noMessagesBody)
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.secondaryText)
                    }
                }
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("cabal-action-chat")
        case .loaded(let message):
            Button(action: openChat) {
                messageCard(message)
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spoken(message))
            .accessibilityHint("Opens chat")
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("cabal-action-chat")
        }
    }

    private func messageCard(_ message: CabalChatPreviewMessage) -> some View {
        MonacoCard(style: .tight) {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                MonacoAvatar(
                    photoURL: message.photoURL, displayName: message.author, size: .preview, seed: message.authorID)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    HStack(spacing: MonacoTheme.Space.s) {
                        Text(message.author)
                            .font(MonacoTheme.Typo.subheadStrong)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .lineLimit(1)
                        Text(RelativeTimeFormatter.label(date: message.createdAt, now: Date()))
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .lineLimit(1)
                    }
                    Text(message.body)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.bodyText)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)
                    Text(CabalCopy.chatFooter(cabalName: cabalName, unread: unread))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .lineLimit(1)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if let badge = CabalCopy.unreadBadge(unread) {
                    Text(badge)
                        .font(MonacoTheme.Typo.captionStrong.monospacedDigit())
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minWidth: SectionHeaderMetrics.countSize, minHeight: SectionHeaderMetrics.countSize)
                        .background(Capsule().fill(MonacoTheme.brandSoft))
                        .accessibilityIdentifier("cabal-action-chat-unread")
                }
                MonacoRowChevron()
                    .frame(minHeight: SectionHeaderMetrics.countSize)
            }
        }
    }

    private var unread: Int { model?.unreadCount ?? 0 }

    private func spoken(_ message: CabalChatPreviewMessage) -> String {
        let age = RelativeTimeFormatter.label(date: message.createdAt, now: Date())
        var parts = ["Chat", "\(message.author), \(age) ago: \(message.body)"]
        if unread > 0 { parts.append(CabalCopy.unreadLabel(unread)) }
        return parts.joined(separator: ". ")
    }
}
