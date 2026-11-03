import MonacoCore
import SwiftUI

struct FShareSticker: View {
    let spec: ShareStickerSpec
    let size: CGSize

    static let designWidth: CGFloat = 360
    static let designHeight: CGFloat = 204

    private var scale: CGFloat { size.width / Self.designWidth }
    private var tone: Color {
        switch spec.tone {
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .neutral: MonacoTheme.primaryText
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs * scale) {
            HStack(spacing: MonacoTheme.Space.s * scale) {
                mark
                Text(spec.name).font(.system(size: 18 * scale, weight: .bold)).foregroundStyle(Color.white)
            }
            if let ticker = spec.ticker {
                HStack(spacing: 8 * scale) {
                    StockMark(symbol: ticker.symbol, size: 24 * scale, logoURL: ticker.logoURL)
                    Text(ticker.detail.map { "\(ticker.symbol) \u{00B7} \($0)" } ?? ticker.symbol)
                        .font(.system(size: 16 * scale, weight: .medium)).foregroundStyle(Color.white)
                }
            }
            HStack(alignment: .lastTextBaseline, spacing: MonacoTheme.Space.titleGap * scale) {
                Text(spec.figureMain)
                    .font(.system(size: 50 * scale, weight: .black).width(.condensed).monospacedDigit())
                    .minimumScaleFactor(0.5)
                    .foregroundStyle(tone)
                if let secondary = spec.figureSecondary {
                    Text(secondary).font(.system(size: 17 * scale, weight: .semibold).monospacedDigit())
                        .foregroundStyle(tone)
                }
            }
            Spacer(minLength: 0)
            wordmark
        }
        .lineLimit(1)
        .padding(MonacoTheme.Space.cardPadding * scale)
        .frame(width: size.width, height: size.height, alignment: .topLeading)
        .background(
            Color.black, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.shareCard * scale, style: .continuous)
        )
        .environment(\.colorScheme, .dark)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spec.spoken)
        .accessibilityIdentifier("share-sticker")
    }

    private var wordmark: some View {
        HStack(alignment: .bottom) {
            Text("monaco").font(.system(size: 32 * scale, weight: .bold)).foregroundStyle(Color.white)
            Spacer()
            if let code = spec.code {
                VStack(alignment: .trailing, spacing: 0) {
                    Text("Join with code").font(.system(size: 13 * scale)).foregroundStyle(MonacoTheme.secondaryText)
                    Text(code.uppercased()).font(.system(size: 16 * scale, weight: .semibold).monospacedDigit())
                        .foregroundStyle(Color.white.opacity(0.9))
                }
            }
        }
    }

    @ViewBuilder
    private var mark: some View {
        switch spec.mark {
        case .crest(let crest):
            CabalMark(
                groupId: crest.cabalID, name: crest.name, size: 40 * scale, pictureUrl: crest.pictureURL?.absoluteString
            )
        case .photo(let url, let name, let crest):
            CabalMark(groupId: name, name: name, size: 40 * scale, pictureUrl: url?.absoluteString, shape: .circle)
                .overlay(alignment: .bottomTrailing) {
                    if let crest {
                        CabalMark(
                            groupId: crest.cabalID, name: crest.name, size: 18 * scale,
                            pictureUrl: crest.pictureURL?.absoluteString
                        )
                        .offset(x: 4 * scale, y: 4 * scale)
                    }
                }
        }
    }
}
