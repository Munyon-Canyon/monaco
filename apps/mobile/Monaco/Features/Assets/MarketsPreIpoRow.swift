import MonacoCore
import SwiftUI

enum MarketsRowMetrics {
    static let logo: CGFloat = 40
    static let height: CGFloat = 64
    static let sparklineWidth: CGFloat = 60
    static let sparklineHeight: CGFloat = 24
}

struct MarketsPreIpoRow: View {
    let asset: MarketAsset
    var isLast = false
    let identifier: String
    let open: (MarketAsset) -> Void

    private var name: String { AssetDisplayName.format(catalogName: asset.name, kind: asset.kind) }

    private var estimateLine: String {
        asset.estimate.map { PreIpoRowCopy.estimateLine(asOf: $0.asOf) } ?? asset.ticker
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
                Text(name)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Text(estimateLine)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    if let estimate = asset.estimate {
                        MoneyText(micros: estimate.priceMicros, style: .row)
                    } else {
                        Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
                    }
                    Text(PreIpoRowCopy.eligibility(tradable: asset.isTradable))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(asset.isTradable ? MonacoTheme.profit : MonacoTheme.secondaryText)
                        .lineLimit(1)
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier(identifier)
    }
}

struct MarketsRowFrame<Leading: View, Labels: View, Trailing: View>: View {
    var isLast = false
    @ViewBuilder let leading: Leading
    @ViewBuilder let labels: Labels
    @ViewBuilder let trailing: Trailing

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            leading
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) { labels }
                .frame(maxWidth: .infinity, alignment: .leading)
            trailing
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(minHeight: MarketsRowMetrics.height)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(
                    .leading, MonacoTheme.Space.gutter + MarketsRowMetrics.logo + MonacoTheme.Space.sm)
            }
        }
    }
}
