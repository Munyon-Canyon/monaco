import MonacoAPI
import MonacoCore
import SwiftUI

struct FeedItemCell: View {
    let item: Components.Schemas.FeedItem

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Capsule()
                .fill(accentColor)
                .frame(width: 3)
                .frame(maxHeight: .infinity)
                .accessibilityHidden(true)
            if let actorID = item.actorId {
                NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: actorID))) {
                    MonacoAvatar(photoURL: nil, displayName: "", size: 40, seed: actorID)
                        .frame(width: 44, height: 44)
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Open profile")
                .accessibilityIdentifier("feed-actor-\(item.id)")
            }
            destinationLink
        }
        .fixedSize(horizontal: false, vertical: true)
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-cell-\(item.id)")
    }

    @ViewBuilder private var destinationLink: some View {
        if let route = FeedDestination(item)?.route {
            NavigationLink(value: route) { content }
                .buttonStyle(.plain)
        } else {
            content
        }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
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
            footer
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }

    private var footer: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(item.createdAt, format: .relative(presentation: .named, unitsStyle: .abbreviated))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Spacer(minLength: 0)
            if item.commentCount > 0 {
                Label("\(item.commentCount)", systemImage: "bubble.left")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityLabel(item.commentCount == 1 ? "1 comment" : "\(item.commentCount) comments")
            }
        }
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
