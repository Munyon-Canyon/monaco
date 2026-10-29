import MonacoCore
import SwiftUI

extension MarketAsset {
    var dayTone: PnLTone {
        guard let basisPoints = changeBasisPoints, basisPoints != 0 else { return .flat }
        return basisPoints > 0 ? .profit : .loss
    }
}

struct MoverTile: View {
    let asset: MarketAsset
    let action: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .title) private var scaledWidth: CGFloat = MoverTile.width
    @ScaledMetric(relativeTo: .title) private var scaledHeight: CGFloat = MoverTile.height

    static let width: CGFloat = 104
    static let height: CGFloat = 152

    private var move: String? {
        asset.changeBasisPoints.map { PercentFormatter.format(basisPoints: $0, signed: true, fractionDigits: 1) }
    }

    private var spokenLabel: String {
        "\(asset.ticker), today \(move.map(PnLSpeech.percent) ?? "unavailable")"
    }

    var body: some View {
        Button(action: action) { tile }
            .buttonStyle(MoverTileStyle())
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spokenLabel)
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("markets-mover-\(asset.symbol)")
    }

    private var tile: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            StockMark(
                symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: 32, logoURL: asset.logoURL)
            Spacer(minLength: MonacoTheme.Space.xs)
            Text(asset.ticker)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
            Text(move ?? "—")
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.signed(move))
                .fixedSize()
            if let series = asset.sparkline, !dynamicTypeSize.isAccessibilitySize {
                Sparkline(
                    series: series, tone: asset.dayTone,
                    width: Self.width - 2 * MonacoTheme.Space.cardPadding, height: 24)
            }
        }
        .padding(MonacoTheme.Space.cardPadding)
        .frame(width: scaledWidth, height: scaledHeight, alignment: .leading)
    }
}

private struct MoverTileStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
        configuration.label
            .background(shape.fill(configuration.isPressed ? MonacoTheme.surfaceSunken : MonacoTheme.surface))
            .contentShape(shape)
    }
}
