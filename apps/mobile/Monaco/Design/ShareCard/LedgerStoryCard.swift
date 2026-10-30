import MonacoCore
import SwiftUI

struct LedgerStoryCard: View {
    let spec: ShareCardSpec
    let size: CGSize
    var progress: Double = 1

    private var scale: CGFloat { size.width / FShareCard.designWidth }
    private var tone: Color { spec.figure.tone == .loss ? MonacoTheme.loss : MonacoTheme.profit }

    var body: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 0) {
                cover
                header
                    .padding(.horizontal, 14 * scale)
                    .padding(.top, 10 * scale)
                figure
                    .padding(.horizontal, 14 * scale)
                    .padding(.top, 6 * scale)
                FShareChart(chart: spec.chart, tone: spec.figure.tone, progress: progress, scale: scale)
                    .frame(maxHeight: .infinity)
                lower
                    .padding(.horizontal, 14 * scale)
                    .padding(.bottom, 14 * scale)
            }
            .frame(maxWidth: .infinity)
            .background(Color.black, in: RoundedRectangle(cornerRadius: 12 * scale, style: .continuous))
            .clipShape(RoundedRectangle(cornerRadius: 12 * scale, style: .continuous))
            footer
        }
        .padding(.horizontal, FShareCard.frameWidth * scale)
        .padding(.top, FShareCard.frameWidth * scale)
        .frame(width: size.width, height: size.height)
        .background(
            MonacoTheme.brandFill,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.shareCard * scale, style: .continuous)
        )
        .environment(\.colorScheme, .dark)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spec.spoken)
        .accessibilityIdentifier("ledger-share-card")
    }

    private var coverHeight: CGFloat { size.height > size.width * 1.5 ? 160 : 64 }

    @ViewBuilder private var cover: some View {
        if case .photo(_, _, let crest?) = spec.header.mark {
            CabalMark(
                groupId: crest.cabalID, name: crest.name, size: size.width, pictureUrl: crest.pictureURL?.absoluteString
            )
            .frame(
                width: size.width - FShareCard.frameWidth * 2 * scale, height: coverHeight * scale, alignment: .center
            )
            .clipped()
        }
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 10 * scale) {
            if case .photo(let url, let name, _) = spec.header.mark {
                CabalMark(groupId: name, name: name, size: 40 * scale, pictureUrl: url?.absoluteString, shape: .circle)
            }
            VStack(alignment: .leading, spacing: 2 * scale) {
                Text(spec.header.title).font(.system(size: 18 * scale, weight: .bold)).foregroundStyle(Color.white)
                Text(spec.header.subtitle).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(2)
            }
            Spacer(minLength: 0)
        }
    }

    private var figure: some View {
        HStack(alignment: .lastTextBaseline, spacing: 6 * scale) {
            Text(spec.figure.main)
                .font(.system(size: 54 * scale, weight: .black).width(.condensed).monospacedDigit())
                .minimumScaleFactor(0.5)
                .lineLimit(1)
                .foregroundStyle(tone)
            if let context = spec.figure.context {
                Text(context).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    private var lower: some View {
        VStack(alignment: .leading, spacing: 8 * scale) {
            if !spec.stats.isEmpty {
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
    }

    private var footer: some View {
        HStack(alignment: .center) {
            MonacoWordmark(.toolbar)
                .fixedSize()
                .scaleEffect(scale, anchor: .leading)
                .foregroundStyle(Color.white)
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
