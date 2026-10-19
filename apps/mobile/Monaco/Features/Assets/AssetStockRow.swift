import MonacoCore
import SwiftUI

struct AssetStockRow: View {
    let asset: MarketAsset
    var isLast = false

    var body: some View {
        MonacoRow(
            title: AssetDisplayName.format(catalogName: asset.name, kind: asset.kind),
            subtitle: asset.isTradable ? asset.ticker : "Can't buy right now",
            chevron: true,
            isLast: isLast
        ) {
            StockMark(
                symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: .row,
                logoURL: asset.logoURL)
        } trailing: {
            AssetRowFigures(asset: asset)
        }
        .opacity(asset.isTradable ? 1 : 0.45)
    }
}

private struct AssetRowFigures: View {
    let asset: MarketAsset

    var body: some View {
        if asset.isPaused { PausedTag() }
        HStack(spacing: MonacoTheme.Space.xs) {
            if !asset.session.isRegularSession {
                Image(systemName: "moon.fill")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.warning)
                    .accessibilityLabel("Market closed")
            }
            if let micros = asset.priceMicros {
                MoneyText(micros: micros, style: .row)
            } else {
                Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        if let basisPoints = asset.changeBasisPoints {
            PercentText(basisPoints: basisPoints, style: .caption)
        } else {
            Text("—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
        }
    }
}

struct AssetRowButton: View {
    let asset: MarketAsset
    var isLast = false
    let identifier: String
    let open: (MarketAsset) -> Void

    var body: some View {
        Button {
            open(asset)
        } label: {
            AssetStockRow(asset: asset, isLast: isLast)
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier(identifier)
    }
}
