import MonacoCore
import SwiftUI

struct MarketsSearch: View {
    let model: MonacoCore.StocksTabModel
    let open: (MarketAsset) -> Void

    var body: some View {
        ScrollView {
            if !model.isSearching {
                Color.clear.frame(height: MonacoTheme.Space.l)
            } else if model.searchResults.isEmpty {
                noRows
            } else {
                results
            }
        }
        .scrollDismissesKeyboard(.interactively)
        .accessibilityIdentifier("assets-grid-search")
    }

    private var results: some View {
        let rows = model.searchResults
        return LazyVStack(spacing: 0) {
            ForEach(rows) { asset in
                row(asset, isLast: asset.id == rows.last?.id)
                    .onAppear {
                        guard model.hasMore, asset.id == rows.last?.id else { return }
                        Task { await model.loadMore() }
                    }
            }
        }
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.l)
    }

    @ViewBuilder private func row(_ asset: MarketAsset, isLast: Bool) -> some View {
        if asset.kind == .preIpo {
            MarketsPreIpoRow(
                asset: asset, isLast: isLast, identifier: "assets-row-\(asset.symbol)", open: open)
        } else {
            MarketsSearchRow(
                asset: asset, lines: model.searchLines(for: asset), isLast: isLast,
                identifier: "assets-row-\(asset.symbol)", open: open)
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
                title: "No results for “\(model.query.trimmingCharacters(in: .whitespacesAndNewlines))”",
                message: "Try a company name or ticker.", systemImage: "magnifyingglass"
            )
            .accessibilityIdentifier("assets-empty")
        }
    }
}

private struct MarketsSearchRow: View {
    let asset: MarketAsset
    let lines: (title: String, subline: String)
    var isLast = false
    let identifier: String
    let open: (MarketAsset) -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var tone: PnLTone {
        guard let basisPoints = asset.changeBasisPoints, basisPoints != 0 else { return .flat }
        return basisPoints > 0 ? .profit : .loss
    }

    var body: some View {
        Button {
            open(asset)
        } label: {
            MarketsRowFrame(isLast: isLast) {
                StockMark(
                    symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind,
                    size: MarketsRowMetrics.logo, logoURL: asset.logoURL)
            } labels: {
                Text(lines.title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Text(lines.subline)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            } trailing: {
                if let series = asset.sparkline, !dynamicTypeSize.isAccessibilitySize {
                    Sparkline(
                        series: series, tone: tone, width: MarketsRowMetrics.sparklineWidth,
                        height: MarketsRowMetrics.sparklineHeight)
                }
                figures
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier(identifier)
    }

    private var figures: some View {
        VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
            if let micros = asset.priceMicros {
                MoneyText(micros: micros, style: .row)
            } else {
                Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
            }
            if let basisPoints = asset.changeBasisPoints {
                change(basisPoints)
            } else {
                Text("—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    private func change(_ basisPoints: Int64) -> some View {
        let text = PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1)
        return Text(text)
            .font(MonacoTheme.Typo.caption)
            .monospacedDigit()
            .foregroundStyle(MonacoTheme.signed(text))
            .accessibilityLabel(PnLSpeech.percent(text))
    }
}
