import MonacoCore
import SwiftUI
import UIKit

struct ShareCardArt {
    var color: Color
    var band: UIImage?
}

extension ShareMark {
    var cabal: ShareCrest? {
        switch self {
        case .crest(let crest): crest
        case .photo(_, _, let crest): crest
        }
    }
}

@MainActor
enum ShareCardArtStore {
    private static var cache: [String: ShareCardArt] = [:]

    private static func key(_ crest: ShareCrest) -> String {
        crest.cabalID + (crest.pictureURL?.absoluteString ?? "")
    }

    static func art(for spec: ShareCardSpec) -> ShareCardArt {
        guard let crest = spec.header.mark.cabal else {
            return ShareCardArt(color: SceneColor.color(forScene: MonacoWorldScene(index: 0)))
        }
        return cache[key(crest)]
            ?? ShareCardArt(color: SceneColor.color(forScene: .forSeed(crest.cabalID)))
    }

    static func resolve(_ spec: ShareCardSpec) async {
        guard let crest = spec.header.mark.cabal, let url = crest.pictureURL else { return }
        let color = await SceneColor.resolve(cabalID: crest.cabalID, pictureURL: url)
        let band = await MonacoRemoteImageStore.avatars.image(for: url)
        cache[key(crest)] = ShareCardArt(color: color, band: band)
    }
}

struct FShareCard: View {
    let spec: ShareCardSpec
    let size: CGSize
    var progress: Double = 1
    var art: ShareCardArt?

    @State private var live: ShareCardArt?

    static let designWidth: CGFloat = 360
    static let frameWidth: CGFloat = 4
    static let bandHeight: CGFloat = 52

    private var scale: CGFloat { size.width / Self.designWidth }
    private var resolvedArt: ShareCardArt { art ?? live ?? ShareCardArtStore.art(for: spec) }
    private var bandPoints: CGFloat {
        let t = min(max((size.height / size.width - 1) / 0.25, 0), 1)
        return Self.bandHeight * (0.3 + 0.7 * t)
    }
    private var cabal: ShareCrest? { spec.header.mark.cabal }
    private var tone: Color {
        switch spec.figure.tone {
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .neutral: MonacoTheme.primaryText
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            body(in: spec)
            footer
        }
        .padding(.horizontal, Self.frameWidth * scale)
        .padding(.top, Self.frameWidth * scale)
        .frame(width: size.width, height: size.height)
        .background(
            resolvedArt.color,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.shareCard * scale, style: .continuous)
        )
        .task(id: spec.header.mark) {
            guard art == nil else { return }
            await ShareCardArtStore.resolve(spec)
            live = ShareCardArtStore.art(for: spec)
        }
        .environment(\.colorScheme, .dark)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spec.spoken)
        .accessibilityIdentifier("share-card")
    }

    private func body(in spec: ShareCardSpec) -> some View {
        let top = cabal == nil ? MonacoTheme.Radius.inner : MonacoTheme.Radius.shareCard - Self.frameWidth
        return VStack(alignment: .leading, spacing: 0) {
            if let cabal {
                band(for: cabal)
            }
            header
                .padding(.horizontal, 14 * scale)
                .padding(.top, 14 * scale)
            FShareChart(chart: spec.chart, tone: spec.figure.tone, progress: progress, scale: scale)
                .frame(maxHeight: size.height * 0.3)
            Spacer(minLength: 0)
            lower
                .padding(.horizontal, 14 * scale)
                .padding(.bottom, 14 * scale)
        }
        .frame(maxWidth: .infinity)
        .background(Color.black)
        .clipShape(
            UnevenRoundedRectangle(
                topLeadingRadius: top * scale, bottomLeadingRadius: MonacoTheme.Radius.inner * scale,
                bottomTrailingRadius: MonacoTheme.Radius.inner * scale, topTrailingRadius: top * scale,
                style: .continuous))
    }

    private func band(for crest: ShareCrest) -> some View {
        let scene = Image(MonacoWorldScene.forSeed(crest.cabalID).imageName)
        let image = resolvedArt.band.map { Image(uiImage: $0) } ?? scene
        return Color.clear
            .frame(height: bandPoints * scale)
            .overlay { image.resizable().aspectRatio(contentMode: .fill) }
            .clipped()
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 10 * scale) {
            if cabal == nil {
                mark
            }
            VStack(alignment: .leading, spacing: 2 * scale) {
                Text(spec.header.title).font(.system(size: 18 * scale, weight: .bold)).foregroundStyle(Color.white)
                Text(spec.header.subtitle).font(.system(size: 14 * scale)).foregroundStyle(MonacoTheme.secondaryText)
                    .minimumScaleFactor(0.7)
            }
            Spacer(minLength: 8 * scale)
            Text(spec.header.dateText).font(.system(size: 14 * scale).monospacedDigit())
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .lineLimit(1)
        .minimumScaleFactor(0.6)
    }

    @ViewBuilder
    private var mark: some View {
        if case .photo(let url, let name, nil) = spec.header.mark {
            CabalMark(groupId: name, name: name, size: 40 * scale, pictureUrl: url?.absoluteString, shape: .circle)
        }
    }

    private var lower: some View {
        VStack(alignment: .leading, spacing: 8 * scale) {
            if !spec.tickerLine.symbol.isEmpty {
                HStack(spacing: 8 * scale) {
                    StockMark(symbol: spec.tickerLine.symbol, size: 24 * scale, logoURL: spec.tickerLine.logoURL)
                    Text(spec.tickerLine.symbol).font(.system(size: 18 * scale, weight: .bold))
                        .foregroundStyle(Color.white)
                    Text(spec.tickerLine.detail).font(.system(size: 14 * scale).monospacedDigit())
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .lineLimit(1)
                .minimumScaleFactor(0.6)
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
            .minimumScaleFactor(0.6)
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
        .minimumScaleFactor(0.6)
        .padding(.horizontal, 10 * scale)
        .frame(height: 62 * scale)
    }
}
