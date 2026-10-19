import MonacoCore
import SwiftUI

struct MarketsMoversSection: View {
    let movers: [MarketAsset]
    let isLoading: Bool
    let open: (MarketAsset) -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Top movers").padding(.horizontal, MonacoTheme.Space.gutter)
            Text("24h token price change")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.sm)
            rail
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-movers")
    }

    @ViewBuilder private var rail: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.sm) { tiles }
                .padding(.horizontal, MonacoTheme.Space.gutter)
        } else {
            ScrollView(.horizontal) {
                LazyHStack(alignment: .top, spacing: MonacoTheme.Space.sm) { tiles }
                    .scrollTargetLayout()
            }
            .scrollTargetBehavior(.viewAligned)
            .contentMargins(.horizontal, MonacoTheme.Space.gutter)
            .scrollIndicators(.hidden)
        }
    }

    @ViewBuilder private var tiles: some View {
        if isLoading {
            ForEach(0..<2, id: \.self) { _ in
                SkeletonBlock(
                    width: MoverTile.width(for: dynamicTypeSize), height: MoverTile.minHeight,
                    radius: MonacoTheme.Radius.cardTight)
            }
        } else {
            ForEach(movers) { asset in
                MoverTile(asset: asset) { open(asset) }
            }
        }
    }
}

struct MarketsWatchlistSection: View {
    let rows: [WatchlistQuotes.Row]
    let open: (String) -> Void
    let manage: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(
                "Watchlist", trailing: "Manage", actionIdentifier: "markets-watchlist-manage", action: manage
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if rows.isEmpty {
                MarketsWatchlistEmpty(add: manage)
            } else {
                ForEach(rows) { row in
                    WatchlistQuoteRow(row: row, isLast: row.id == rows.last?.id, open: open)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-watchlist")
    }
}

private struct WatchlistQuoteRow: View {
    let row: WatchlistQuotes.Row
    let isLast: Bool
    let open: (String) -> Void

    var body: some View {
        switch row {
        case .quoted(let asset):
            AssetRowButton(
                asset: asset, isLast: isLast, identifier: "markets-watchlist-row-\(asset.symbol)",
                open: { open($0.symbol) })
        case .loading:
            StockRowSkeleton(rows: 1)
        case .unavailable(let symbol):
            Button {
                open(symbol)
            } label: {
                MonacoRow(
                    title: AssetSymbolFormatter.display(symbol, kind: .stock), chevron: true, isLast: isLast
                ) {
                    StockMark(symbol: symbol, size: .row)
                } trailing: {
                    Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
                    Text("—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("markets-watchlist-row-\(symbol)")
        }
    }
}

private struct MarketsWatchlistEmpty: View {
    let add: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            SunkenGlyphMark(systemImage: "star", size: 36)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text("Your market, at a glance")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text("Add stocks to your watchlist.")
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

struct MarketsPopularSection: View {
    let rows: [MarketAsset]
    let isLoading: Bool
    let open: (MarketAsset) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Popular now").padding(.horizontal, MonacoTheme.Space.gutter)
            Text("Most popular on Monaco")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.sm)
            if isLoading {
                StockRowSkeleton(rows: StocksTabModel.popularNowLimit)
            } else {
                ForEach(rows) { asset in
                    AssetRowButton(
                        asset: asset, isLast: asset.id == rows.last?.id,
                        identifier: "markets-popular-row-\(asset.symbol)", open: open)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-popular")
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

struct MarketsAllStocks: View {
    let model: MonacoCore.StocksTabModel
    let title: String
    let open: (MarketAsset) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(title).padding(.horizontal, MonacoTheme.Space.gutter)
            MarketsSortFilterRow(browse: model.browse) { next in Task { await model.show(next) } }
                .padding(.bottom, MonacoTheme.Space.s)
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
            if model.isSearching {
                EmptyState(
                    title: "No matching stocks", message: "Try a company name, ticker or another filter.",
                    systemImage: "magnifyingglass"
                )
                .accessibilityIdentifier("assets-search-empty")
            } else {
                EmptyState(
                    title: "No stocks in this filter", message: "Try a different category.",
                    systemImage: "line.3.horizontal.decrease"
                )
                .accessibilityIdentifier("assets-filter-empty")
            }
        }
    }
}
