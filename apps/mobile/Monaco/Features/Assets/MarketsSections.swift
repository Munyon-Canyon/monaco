import MonacoCore
import SwiftUI

struct MarketsWatchlistSection: View {
    let model: MonacoCore.StocksTabModel
    let rows: [WatchlistQuotes.Row]
    var showsSeeAll = true
    let open: (MarketAsset) -> Void
    let openSymbol: (String) -> Void
    let seeAll: () -> Void
    let manage: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(
                "Watchlist", count: model.watchlistCount, trailing: showsSeeAll && !rows.isEmpty ? "See all" : nil,
                actionIdentifier: "markets-watchlist-see-all", action: seeAll
            )
            .secondaryCount()
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if rows.isEmpty {
                MarketsWatchlistEmpty(add: manage)
            } else {
                ForEach(rows) { row in
                    MarketsWatchlistRow(
                        model: model, row: row, isLast: row.id == rows.last?.id, open: open, openSymbol: openSymbol)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-watchlist")
    }
}

private struct MarketsWatchlistRow: View {
    let model: MonacoCore.StocksTabModel
    let row: WatchlistQuotes.Row
    let isLast: Bool
    let open: (MarketAsset) -> Void
    let openSymbol: (String) -> Void

    var body: some View {
        switch row {
        case .quoted(let asset) where asset.kind == .preIpo:
            MarketsPreIpoRow(
                asset: asset, isLast: isLast, identifier: "markets-watchlist-row-\(asset.symbol)", open: open)
        case .quoted(let asset):
            MarketsWatchRow(
                asset: asset, subline: model.subline(for: asset), isLast: isLast,
                identifier: "markets-watchlist-row-\(asset.symbol)", open: open)
        case .loading:
            StockRowSkeleton(rows: 1)
        case .unavailable(let symbol):
            Button {
                openSymbol(symbol)
            } label: {
                MarketsRowFrame(isLast: isLast) {
                    StockMark(symbol: symbol, size: MarketsRowMetrics.logo)
                } labels: {
                    Text(AssetSymbolFormatter.display(symbol, kind: .stock))
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.primaryText)
                } trailing: {
                    Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("markets-watchlist-row-\(symbol)")
        }
    }
}

struct MarketsPreIpoSection: View {
    let rows: [MarketAsset]
    var showsSeeAll = true
    let open: (MarketAsset) -> Void
    let seeAll: () -> Void
    let info: () -> Void
    var onLastAppear: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(
                MarketsInfoCopy.preIpoTitle, trailing: showsSeeAll ? "See all" : nil,
                actionIdentifier: "markets-preipo-see-all", action: seeAll
            )
            .info(identifier: "markets-preipo-info", action: info)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            LazyVStack(spacing: 0) {
                ForEach(rows) { asset in
                    MarketsPreIpoRow(
                        asset: asset, isLast: asset.id == rows.last?.id,
                        identifier: "markets-preipo-row-\(asset.symbol)", open: open
                    )
                    .onAppear { if asset.id == rows.last?.id { onLastAppear?() } }
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-preipo")
    }
}

struct MarketsPreIpoInfoSheet: View {
    var body: some View {
        Text(MarketsInfoCopy.preIpo)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("markets-preipo-info-body")
            .monacoFittedSheet(title: MarketsInfoCopy.preIpoTitle, showsClose: true)
    }
}

struct MarketsSearchResults: View {
    let model: MonacoCore.StocksTabModel
    let open: (MarketAsset) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Search results").padding(.horizontal, MonacoTheme.Space.gutter)
            if model.rows.isEmpty {
                noRows
            } else {
                LazyVStack(spacing: 0) {
                    ForEach(model.rows) { asset in
                        AssetRowButton(
                            asset: asset, isLast: asset.id == model.rows.last?.id,
                            identifier: "assets-row-\(asset.symbol)", open: open
                        )
                        .onAppear {
                            guard model.hasMore, asset.id == model.rows.last?.id else { return }
                            Task { await model.loadMore() }
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder private var noRows: some View {
        switch model.phase {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: false)
        case .failed:
            MonacoErrorRow(thing: "stocks", identifier: "assets-failed") { Task { await model.load() } }
        case .loaded:
            EmptyState(
                title: "No matching stocks", message: "Try a company name, ticker or another filter.",
                systemImage: "magnifyingglass"
            )
            .accessibilityIdentifier("assets-search-empty")
        }
    }
}

struct MarketsWatchlistEmpty: View {
    let add: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            SunkenGlyphMark(systemImage: "star", size: 36)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text("Your market, at a glance")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text("Star a stock to watch it here.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button("Add", action: add)
                .buttonStyle(.monacoCompact)
                .accessibilityIdentifier("markets-watchlist-add")
        }
        .padding(MonacoTheme.Space.sm)
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
                .strokeBorder(MonacoTheme.border, lineWidth: 1)
        )
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

struct MarketsLoading: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonTitle()
            SkeletonFraction(0.4) { SkeletonBar() }
            SkeletonCardBlock()
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: false)
                .monacoBleed()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityHidden(true)
    }
}
