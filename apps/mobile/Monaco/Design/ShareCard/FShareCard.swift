import MonacoCore
import SwiftUI

struct FShareCard: View {
    let spec: ShareCardSpec
    let size: CGSize
    var progress: Double = 1

    static let designWidth: CGFloat = 360
    static let frameWidth: CGFloat = 4

    private var scale: CGFloat { size.width / Self.designWidth }
    private var tone: Color { spec.figure.tone == .gain ? MonacoTheme.profit : MonacoTheme.loss }

    var body: some View {
        VStack(spacing: 0) {
            body(in: spec)
            footer
        }
        .padding(.horizontal, Self.frameWidth * scale)
        .padding(.top, Self.frameWidth * scale)
        .frame(width: size.width, height: size.height)
        .background(
            MonacoTheme.brandFill,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.shareCard * scale, style: .continuous)
        )
        .environment(\.colorScheme, .dark)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spec.spoken)
        .accessibilityIdentifier("share-card")
    }

    private func body(in spec: ShareCardSpec) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            header
                .padding(.horizontal, 14 * scale)
                .padding(.top, 14 * scale)
            FShareChart(chart: spec.chart, tone: spec.figure.tone, progress: progress, scale: scale)
                .frame(maxHeight: .infinity)
            lower
                .padding(.horizontal, 14 * scale)
                .padding(.bottom, 14 * scale)
        }
        .frame(maxWidth: .infinity)
        .background(
            Color.black,
            in: UnevenRoundedRectangle(
                topLeadingRadius: 12 * scale, bottomLeadingRadius: 12 * scale, bottomTrailingRadius: 12 * scale,
                topTrailingRadius: 12 * scale, style: .continuous))
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 10 * scale) {
            mark
            VStack(alignment: .leading, spacing: 2 * scale) {
                Text(spec.header.title).font(.system(size: 18 * scale, weight: .bold)).foregroundStyle(Color.white)
                Text(spec.header.subtitle).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 8 * scale)
            Text(spec.header.dateText).font(.system(size: 14 * scale).monospacedDigit())
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .lineLimit(1)
    }

    @ViewBuilder
    private var mark: some View {
        switch spec.header.mark {
        case .crest(let crest):
            CabalMark(
                groupId: crest.cabalID, name: crest.name, size: 40 * scale, pictureUrl: crest.pictureURL?.absoluteString
            )
        case .photo(let url, let name, _):
            CabalMark(groupId: name, name: name, size: 40 * scale, pictureUrl: url?.absoluteString, shape: .circle)
        }
    }

    private var lower: some View {
        VStack(alignment: .leading, spacing: 8 * scale) {
            HStack(spacing: 8 * scale) {
                StockMark(symbol: spec.tickerLine.symbol, size: 24 * scale, logoURL: spec.tickerLine.logoURL)
                Text(spec.tickerLine.symbol).font(.system(size: 18 * scale, weight: .bold))
                    .foregroundStyle(Color.white)
                Text(spec.tickerLine.detail).font(.system(size: 14 * scale).monospacedDigit())
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            HStack(alignment: .lastTextBaseline, spacing: 6 * scale) {
                Text(spec.figure.main)
                    .font(.system(size: 54 * scale, weight: .black).width(.condensed).monospacedDigit())
                    .minimumScaleFactor(0.5)
                    .lineLimit(1)
                    .foregroundStyle(tone)
                if let secondary = spec.figure.secondary {
                    Text(secondary).font(.system(size: 17 * scale, weight: .semibold).monospacedDigit())
                        .foregroundStyle(tone)
                }
                if let context = spec.figure.context {
                    Text(context).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            Rectangle().fill(MonacoTheme.hairline).frame(height: 1)
            HStack(alignment: .top, spacing: 0) {
                ForEach(Array(spec.stats.enumerated()), id: \.offset) { _, stat in
                    VStack(alignment: .leading, spacing: 2 * scale) {
                        Text(stat.label).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
                        Text(stat.value).font(.system(size: 18 * scale, weight: .semibold).monospacedDigit())
                            .foregroundStyle(Color.white)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .lineLimit(1)
        }
    }

    private var footer: some View {
        HStack(alignment: .center) {
            Text("monaco").font(.system(size: 32 * scale, weight: .bold)).foregroundStyle(Color.white)
            Spacer()
            if let code = spec.code {
                VStack(alignment: .trailing, spacing: 0) {
                    Text("Join with code").font(.system(size: 14 * scale)).foregroundStyle(Color.white.opacity(0.7))
                    Text(code).font(.system(size: 18 * scale, weight: .semibold)).tracking(1.5 * scale)
                        .foregroundStyle(Color.white.opacity(0.9))
                }
            }
        }
        .lineLimit(1)
        .padding(.horizontal, 10 * scale)
        .frame(height: 62 * scale)
    }
}
