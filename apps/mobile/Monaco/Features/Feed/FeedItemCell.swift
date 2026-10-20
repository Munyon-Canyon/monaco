import MonacoAPI
import MonacoCore
import SwiftUI

struct FeedItemCell: View {
    let item: Components.Schemas.FeedItem
    let onLike: () -> Void

    var body: some View {
        let event = FeedEvent(item)
        MonacoCard(style: .tight) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                FeedAuthorRow(
                    item: item, event: event, authorReturn: FeedReturnFacts(FeedFigureFormat.authorReturn(item)))
                block(for: event)
                FeedReactionsRow(item: item, onLike: onLike)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-cell-\(item.id)")
    }

    @ViewBuilder private func block(for event: FeedEvent) -> some View {
        switch event.block {
        case .asset(let side):
            let facts = FeedAssetFacts(item, event: event)
            FeedAssetBlock(
                tone: side == .buy ? .buy : .sell, symbol: item.symbol ?? "", ticker: facts.ticker,
                company: facts.company,
                trailing: facts.amount.map { .init(text: $0, color: MonacoTheme.primaryText) },
                line: .init(
                    leading: facts.detail, trailing: facts.status,
                    trailingColor: event.statusColor),
                thesis: facts.thesis, logoURL: item.assetLogoURL, route: item.opensRoute,
                spoken: facts.spoken(event))
        case .price:
            let facts = FeedPriceFacts(item)
            FeedAssetBlock(
                tone: .price, symbol: item.symbol ?? "", ticker: facts.ticker, company: facts.company,
                trailing: facts.change.map {
                    .init(text: $0, color: MonacoTheme.signed($0), caption: "24h change")
                },
                line: .init(leading: "24h token volume", trailing: "Opens soon"),
                logoURL: item.assetLogoURL, route: item.opensRoute, spoken: facts.spoken)
        case .quote:
            FeedReplyBlock(
                caption: item.detail, quote: item.body ?? item.title, route: item.opensRoute)
        case .cabal:
            FeedCabalBlock(item: item, route: item.opensRoute)
        }
    }
}

extension Components.Schemas.FeedItem {
    var assetLogoURL: URL? { CabalMark.resolvedURL(assetLogoUrl) }
    var cabalPictureURL: URL? { CabalMark.resolvedURL(cabalPictureUrl) }

    var opensRoute: AnyAppRoute {
        FeedDestination(self)?.route ?? AnyAppRoute(FeedItemRoute(itemID: id))
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
