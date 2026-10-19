import MonacoCore
import SwiftUI

struct MoverTile: View {
    let asset: MarketAsset
    let action: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static let regularWidth: CGFloat = 244
    static let largeWidth: CGFloat = 285
    static let minHeight: CGFloat = 106

    static func width(for size: DynamicTypeSize) -> CGFloat? {
        if size.isAccessibilitySize { return nil }
        return size >= .xxxLarge ? largeWidth : regularWidth
    }

    private var name: String { AssetDisplayName.format(catalogName: asset.name, kind: asset.kind) }

    private var volumeText: String? { asset.volume24hMicros.map(UsdAmountFormatter.compact(micros:)) }

    private var spokenLabel: String {
        let change = asset.changeBasisPoints.map {
            PnLSpeech.percent(PercentFormatter.format(basisPoints: $0, signed: true))
        }
        let volume = volumeText ?? "unavailable"
        return "\(name), \(asset.ticker), 24 hour price change \(change ?? "unavailable"), 24 hour volume \(volume)"
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
        VStack(spacing: MonacoTheme.Space.sm) {
            topRow
            MonacoRule()
            HStack {
                Text("24h volume").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                Spacer(minLength: MonacoTheme.Space.xs)
                Text(volumeText ?? "—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .padding(MonacoTheme.Space.m)
        .frame(width: Self.width(for: dynamicTypeSize), alignment: .leading)
        .frame(maxWidth: Self.width(for: dynamicTypeSize) == nil ? .infinity : nil, minHeight: Self.minHeight)
    }

    private var topRow: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.s) {
                mark
                labels
                Spacer(minLength: MonacoTheme.Space.xs)
                change
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.s) {
                    mark
                    labels
                }
                change
            }
        }
        .frame(minHeight: 38)
    }

    private var mark: some View {
        StockMark(
            symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: 32, logoURL: asset.logoURL)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(name).font(MonacoTheme.Typo.captionStrong).foregroundStyle(MonacoTheme.primaryText)
            Text(asset.ticker).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
        }
        .lineLimit(1)
    }

    @ViewBuilder private var change: some View {
        if let basisPoints = asset.changeBasisPoints {
            PercentText(basisPoints: basisPoints, style: .caption)
        }
    }
}

private struct MoverTileStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
        configuration.label
            .background(shape.fill(configuration.isPressed ? MonacoTheme.surfaceSunken : MonacoTheme.surface))
            .overlay(shape.strokeBorder(MonacoTheme.border, lineWidth: 1))
            .contentShape(shape)
    }
}
