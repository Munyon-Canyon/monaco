import MonacoCore
import SwiftUI

struct AssetDetailSeedHeader: View {
    let symbol: String
    let seed: AssetSeed

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(symbol: symbol, displayName: seed.name, assetKind: seed.kind, size: 48, logoURL: seed.logoURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(seed.name)
                        .font(MonacoTheme.Typo.section)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(seed.ticker)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                Spacer(minLength: 0)
            }
            .padding(.bottom, MonacoTheme.Space.m)
            if let micros = seed.priceMicros {
                Text(UsdAmountFormatter.format(micros: micros))
                    .moneyFont(.hero)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
            }
            if let basisPoints = seed.changeBasisPoints {
                HStack(spacing: MonacoTheme.Space.sm) {
                    PercentText(basisPoints: basisPoints, style: .row)
                    Text("Past day · \(seed.ticker)")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .padding(.top, MonacoTheme.Space.s)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("asset-detail-seed-header")
    }
}
