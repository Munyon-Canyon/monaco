import MonacoCore
import SwiftUI

struct MarketsWatchRow: View {
    let asset: MarketAsset
    let subline: String
    var isLast = false
    let identifier: String
    let open: (MarketAsset) -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var name: String { AssetDisplayName.format(catalogName: asset.name, kind: asset.kind) }

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
                Text(name)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .fixedSize(horizontal: false, vertical: true)
                Text(subline)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
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
                PercentText(basisPoints: basisPoints, style: .caption)
            } else {
                Text("—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }
}
