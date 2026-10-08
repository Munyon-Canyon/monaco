import MonacoAPI
import MonacoCore
import SwiftUI

struct FeedItemCell: View {
    let item: Components.Schemas.FeedItem
    var opensComments = true

    static func separatorLeadingInset(for item: Components.Schemas.FeedItem) -> CGFloat {
        let stripe = MonacoTheme.Space.m + 3 + MonacoTheme.Space.sm
        return item.actorId == nil ? stripe : stripe + MonacoRowLayout.baseMarkSize + MonacoTheme.Space.sm
    }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Capsule()
                .fill(accentColor)
                .frame(width: 3)
                .frame(maxHeight: .infinity)
                .accessibilityHidden(true)
            if let actorID = item.actorId {
                NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: actorID))) {
                    MonacoAvatar(photoURL: nil, displayName: "", seed: actorID)
                        .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Open profile")
                .accessibilityIdentifier("feed-actor-\(item.id)")
            }
            column
        }
        .fixedSize(horizontal: false, vertical: true)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-cell-\(item.id)")
    }

    private var isProposal: Bool {
        FeedKind(rawValue: item.kind) == .proposal
    }

    private var column: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            if isProposal || !opensComments {
                destination {
                    text
                    footer(commentsLink: false)
                }
            } else {
                destination { text }
                footer(commentsLink: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private func destination(@ViewBuilder _ label: () -> some View) -> some View {
        let label = VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) { label() }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        if let route = FeedDestination(item)?.route {
            NavigationLink(value: route) { label }
                .buttonStyle(.plain)
        } else {
            label
        }
    }

    @ViewBuilder private var text: some View {
        Text(item.title)
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
        if let detail = item.detail, !detail.isEmpty {
            Text(detail)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.muted)
        }
        if let body = item.body, !body.isEmpty {
            Text(body)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(3)
        }
    }

    private func footer(commentsLink: Bool) -> some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(item.createdAt, format: .relative(presentation: .named, unitsStyle: .abbreviated))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Spacer(minLength: 0)
            if commentsLink {
                NavigationLink(value: AnyAppRoute(FeedItemRoute(itemID: item.id))) { commentCount }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("feed-comments-\(item.id)")
            } else {
                commentCount
                    .accessibilityIdentifier("feed-comments-\(item.id)")
            }
        }
    }

    private var commentCount: some View {
        Label("\(item.commentCount)", systemImage: "bubble.left")
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .frame(minWidth: 44, minHeight: 44, alignment: .trailing)
            .contentShape(Rectangle())
            .accessibilityLabel(item.commentCount == 1 ? "1 comment" : "\(item.commentCount) comments")
    }

    private var accentColor: Color {
        switch FeedAccent(item) {
        case .ink: MonacoTheme.ink
        case .positive: MonacoTheme.profit
        case .negative: MonacoTheme.loss
        case .muted: MonacoTheme.muted
        }
    }
}

extension FeedDestination {
    var route: AnyAppRoute {
        switch self {
        case .proposal(let id): AnyAppRoute(ProposalRoute(proposalID: id))
        case .transaction(let cabalID, let transactionID):
            AnyAppRoute(TransactionRoute(cabalID: cabalID, transactionID: transactionID))
        case .asset(let symbol): AnyAppRoute(AssetRoute(symbol: symbol))
        case .cabal(let id): AnyAppRoute(CabalRoute(id: id))
        }
    }
}
