import MonacoAPI
import MonacoCore
import SwiftUI

struct MarketsHeldList: View {
    let model: HeldModel
    let openSymbol: (String) -> Void

    var body: some View {
        switch model.state {
        case .idle, .loading:
            StockRowSkeleton(rows: 4)
        case .failed:
            AssetsLoadFailure(identifier: "markets-held-failed") { Task { await model.load() } }
        case .loaded(let list) where list.rows.isEmpty:
            EmptyState(title: "Your cabals don't hold any stocks yet.", systemImage: "chart.line.uptrend.xyaxis")
                .accessibilityIdentifier("markets-held-empty")
        case .loaded(let list):
            content(list)
        }
    }

    private func content(_ list: HeldList) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Held by your cabals", count: list.rows.count)
                .secondaryCount()
                .padding(.horizontal, MonacoTheme.Space.gutter)
            Text(list.totalLine)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.s)
                .accessibilityIdentifier("markets-held-total")
            LazyVStack(spacing: 0) {
                ForEach(list.rows) { row in
                    MarketsHeldRow(row: row, isLast: row.id == list.rows.last?.id, open: openSymbol)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-held")
    }
}

private struct MarketsHeldRow: View {
    let row: HeldRowFacts
    let isLast: Bool
    let open: (String) -> Void

    private var tone: PnLTone {
        guard let text = row.returnText else { return .flat }
        return text.hasPrefix("+") ? .profit : text.hasPrefix("\u{2212}") ? .loss : .flat
    }

    private var label: String {
        let parts = [row.title, row.subtitle, row.valueText, row.valueNote ?? row.returnText]
        return parts.compactMap { $0 }.joined(separator: ", ")
    }

    var body: some View {
        Button {
            open(row.symbol)
        } label: {
            MarketsRowFrame(isLast: isLast) {
                StockMark(
                    symbol: row.symbol, displayName: row.title, assetKind: row.kind, size: MarketsRowMetrics.logo,
                    logoURL: row.logoURL)
            } labels: {
                Text(row.title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .fixedSize(horizontal: false, vertical: true)
                Text(row.subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    Text(row.valueText).moneyFigure(.row).foregroundStyle(MonacoTheme.ink)
                    if let note = row.valueNote {
                        Text(note).moneyFigure(.caption).foregroundStyle(MonacoTheme.secondaryText)
                    } else if let returnText = row.returnText {
                        Text(returnText).moneyFigure(.caption).foregroundStyle(tone.color)
                    }
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityIdentifier("markets-held-row-\(row.id)")
    }
}
