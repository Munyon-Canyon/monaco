import MonacoCore
import SwiftUI
import UIKit

struct ShareExportView: View {
    let spec: ShareCardSpec
    let format: ShareFormat
    var eyes = ShareEyes()
    let size: CGSize
    var progress: Double = 1

    static let storyInset: CGFloat = 60

    private var unit: CGFloat { size.width / ShareImageExporter.exportWidth }

    var body: some View {
        switch format {
        case .post, .square:
            card(size: size)
        case .sticker:
            FShareSticker(spec: ShareStickerSpec(from: spec, eyes: eyes), size: size)
        case .story:
            story
        }
    }

    private var story: some View {
        let cardWidth = size.width - Self.storyInset * 2 * unit
        let cardSize = CGSize(width: cardWidth, height: cardWidth * (spec.facts.isLedger ? 16.0 / 9.0 : 5.0 / 4.0))
        return VStack(spacing: 0) {
            if !spec.facts.isLedger {
                Text("monaco")
                    .font(.system(size: 88 * unit, weight: .bold))
                    .foregroundStyle(Color.white)
                Color.clear.frame(height: 56 * unit)
            }
            card(size: cardSize)
        }
        .frame(width: size.width, height: size.height)
        .background(Color.black)
        .environment(\.colorScheme, .dark)
    }

    @ViewBuilder private func card(size: CGSize) -> some View {
        if spec.facts.isLedger {
            LedgerStoryCard(spec: spec, size: size, progress: progress)
        } else {
            FShareCard(spec: spec, size: size, progress: progress)
        }
    }
}

@MainActor
enum ShareImageExporter {
    static let exportWidth: CGFloat = 1080

    static func pixelSize(_ format: ShareFormat) -> CGSize {
        switch format {
        case .post: CGSize(width: exportWidth, height: 1350)
        case .sticker:
            CGSize(width: exportWidth, height: FShareSticker.designHeight * exportWidth / FShareSticker.designWidth)
        case .square: CGSize(width: exportWidth, height: exportWidth)
        case .story: CGSize(width: exportWidth, height: 1920)
        }
    }

    static func render(_ spec: ShareCardSpec, format: ShareFormat, eyes: ShareEyes = ShareEyes()) -> UIImage {
        let size = pixelSize(format)
        let opaque = format != .sticker
        let view = ShareExportView(spec: spec, format: format, eyes: eyes, size: size)
            .environment(\.colorScheme, .dark)
            .dynamicTypeSize(.large)
        let renderer = ImageRenderer(content: view)
        renderer.proposedSize = ProposedViewSize(size)
        renderer.scale = 1
        renderer.isOpaque = opaque
        return renderer.uiImage ?? blank(size, opaque: opaque)
    }

    static func warmCache(for spec: ShareCardSpec) async {
        let avatars = avatarURLs(in: spec)
        let logos = [spec.tickerLine.logoURL].compactMap { $0 }
        await withTaskGroup(of: Void.self) { group in
            for url in avatars {
                group.addTask { _ = await MonacoRemoteImageStore.avatars.image(for: url) }
            }
            for url in logos {
                group.addTask { _ = await MonacoRemoteImageStore.stockLogos.image(for: url) }
            }
        }
    }

    static func avatarURLs(in spec: ShareCardSpec) -> [URL] {
        switch spec.header.mark {
        case .crest(let crest): [crest.pictureURL].compactMap { $0 }
        case .photo(let url, _, let crest): [url, crest?.pictureURL].compactMap { $0 }
        }
    }

    private static func blank(_ size: CGSize, opaque: Bool) -> UIImage {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = opaque
        return UIGraphicsImageRenderer(size: size, format: format).image { context in
            guard opaque else { return }
            UIColor.black.setFill()
            context.fill(CGRect(origin: .zero, size: size))
        }
    }
}
