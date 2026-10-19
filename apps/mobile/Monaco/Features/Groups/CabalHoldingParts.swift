import MonacoCore
import SwiftUI

enum CabalHoldingMetrics {
    static let signTile: CGFloat = 35
    static let marketMark: CGFloat = 42
    static let detailsTop: CGFloat = 20
    static let skeletonRows = 4
    static let skeletonCircle: CGFloat = 42
    static let skeletonTitleFraction: CGFloat = 0.6
    static let skeletonBarFraction: CGFloat = 0.4
    static let skeletonChart: CGFloat = 126
}

extension CabalHolding.Tone {
    var color: Color {
        switch self {
        case .neutral: MonacoTheme.primaryText
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .missing: MonacoTheme.secondaryText
        }
    }
}

struct HoldingStatGrid: View {
    let stats: [CabalHolding.Stat]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let columns = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        VStack(spacing: 0) {
            ForEach(Array(Self.rows(stats, columns: columns).enumerated()), id: \.offset) { _, row in
                HStack(alignment: .top, spacing: StatMetrics.columnGap) {
                    ForEach(row) { stat in cell(stat) }
                    if row.count < columns {
                        Color.clear.frame(maxWidth: .infinity, maxHeight: 0)
                    }
                }
            }
        }
        .accessibilityIdentifier("holding-stats")
    }

    static func rows(_ stats: [CabalHolding.Stat], columns: Int) -> [[CabalHolding.Stat]] {
        stride(from: 0, to: stats.count, by: columns).map { Array(stats[$0..<min($0 + columns, stats.count)]) }
    }

    private func cell(_ stat: CabalHolding.Stat) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(stat.label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(stat.value)
                .moneyFont(.row)
                .foregroundStyle(stat.tone.color)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
        }
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(
            maxWidth: .infinity, minHeight: StatMetrics.cellMinHeight, maxHeight: .infinity, alignment: .topLeading
        )
        .overlay(alignment: .bottom) { MonacoRule() }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(stat.spoken)
        .accessibilityIdentifier("holding-stat-\(stat.id)")
    }
}

struct HoldingTradeRow: View {
    let trade: HoldingTrade

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var tone: Color { trade.isBuy ? MonacoTheme.profit : MonacoTheme.loss }

    var body: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.s) {
            signTile
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    labels
                    total
                }
            } else {
                labels
                total
            }
            Image(systemName: "chevron.right")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
        }
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) { MonacoRule() }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(trade.spoken)
        .accessibilityIdentifier("holding-trade-\(trade.id)")
    }

    private var signShape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MarkGeometry.radius(for: CabalHoldingMetrics.signTile), style: .continuous)
    }

    private var signTile: some View {
        Image(systemName: trade.isBuy ? MonacoIcon.down.symbol : MonacoIcon.up.symbol)
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(tone)
            .frame(width: CabalHoldingMetrics.signTile, height: CabalHoldingMetrics.signTile)
            .background(signShape.fill(trade.isBuy ? MonacoTheme.buyWash : MonacoTheme.sellWash))
            .accessibilityHidden(true)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(trade.title)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(trade.dateText)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            if let line = trade.priceLine {
                Text(line)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var total: some View {
        MoneyText(micros: trade.totalMicros, style: .caption, color: MonacoTheme.primaryText)
    }
}

struct HoldingDetails: View {
    let holding: CabalHolding
    let openMarket: () -> Void

    @State private var isExpanded = false

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            VStack(spacing: 0) {
                ReceiptLine(label: CabalHoldingCopy.Detail.asset, value: .words(holding.ticker))
                ReceiptLine(label: CabalHoldingCopy.Detail.scope, value: .words(holding.cabalName))
                ReceiptLine(
                    label: CabalHoldingCopy.Detail.averageBuy, value: .words(CabalHoldingCopy.Detail.averageBuyValue))
                ReceiptLine(
                    label: CabalHoldingCopy.Detail.pnlScope, value: .words(CabalHoldingCopy.Detail.pnlScopeValue),
                    isLast: false)
                marketRow
            }
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(CabalHoldingCopy.details)
                    .foregroundStyle(isExpanded ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
                Image(systemName: MonacoIcon.info.symbol).accessibilityHidden(true)
            }
        }
        .disclosureGroupStyle(.monaco)
        .padding(.top, CabalHoldingMetrics.detailsTop)
        .accessibilityIdentifier("holding-details")
    }

    private var marketRow: some View {
        Button(action: openMarket) {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(
                    symbol: holding.symbol, displayName: holding.assetName, assetKind: holding.assetKind,
                    size: CabalHoldingMetrics.marketMark, logoURL: holding.logoURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(CabalHoldingCopy.viewMarket)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(holding.assetName)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                Image(systemName: "chevron.right")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
            }
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(maxWidth: .infinity, minHeight: MonacoRowLayout.minHeight, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("holding-view-market")
    }
}

struct HoldingSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonFraction(CabalHoldingMetrics.skeletonTitleFraction) {
                SkeletonBlock(height: SkeletonMetrics.titleHeight, radius: SkeletonMetrics.titleRadius)
            }
            SkeletonFraction(CabalHoldingMetrics.skeletonBarFraction) {
                SkeletonBlock(height: SkeletonMetrics.barHeight, radius: SkeletonMetrics.barRadius)
            }
            SkeletonBlock(height: CabalHoldingMetrics.skeletonChart, radius: MonacoTheme.Radius.card)
            ForEach(0..<CabalHoldingMetrics.skeletonRows, id: \.self) { _ in
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: CabalHoldingMetrics.skeletonCircle, height: CabalHoldingMetrics.skeletonCircle,
                        radius: CabalHoldingMetrics.skeletonCircle / 2)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonBar(width: 180)
                        SkeletonBar(width: 120)
                    }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading holding")
        .accessibilityIdentifier("holding-loading")
    }
}
