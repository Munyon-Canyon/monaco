import MonacoCore
import SwiftUI
import UIKit

struct ShareExportView: View {
    let spec: ShareCardSpec
    let format: ShareFormat
    let size: CGSize
    var progress: Double = 1

    static let storyInset: CGFloat = 60

    private var unit: CGFloat { size.width / ShareImageExporter.exportWidth }

    var body: some View {
        switch format {
        case .post, .square, .sticker:
            FShareCard(spec: spec, size: size, progress: progress)
        case .story:
            story
        }
    }

    private var story: some View {
        let cardWidth = size.width - Self.storyInset * 2 * unit
        let cardSize = CGSize(width: cardWidth, height: cardWidth * 5 / 4)
        return VStack(spacing: 0) {
            Text("monaco")
                .font(.system(size: 88 * unit, weight: .bold))
                .foregroundStyle(Color.white)
            Color.clear.frame(height: 56 * unit)
            FShareCard(spec: spec, size: cardSize, progress: progress)
        }
        .frame(width: size.width, height: size.height)
        .background(Color.black)
        .environment(\.colorScheme, .dark)
    }
}

@MainActor
enum ShareImageExporter {
    static let exportWidth: CGFloat = 1080

    static func pixelSize(_ format: ShareFormat) -> CGSize {
        switch format {
        case .post, .sticker: CGSize(width: exportWidth, height: 1350)
        case .square: CGSize(width: exportWidth, height: exportWidth)
        case .story: CGSize(width: exportWidth, height: 1920)
        }
    }

    static func render(_ spec: ShareCardSpec, format: ShareFormat) -> UIImage {
        let size = pixelSize(format)
        let view = ShareExportView(spec: spec, format: format, size: size)
            .environment(\.colorScheme, .dark)
            .dynamicTypeSize(.large)
        let renderer = ImageRenderer(content: view)
        renderer.proposedSize = ProposedViewSize(size)
        renderer.scale = 1
        renderer.isOpaque = true
        return renderer.uiImage ?? blank(size)
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

    private static func blank(_ size: CGSize) -> UIImage {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = true
        return UIGraphicsImageRenderer(size: size, format: format).image { context in
            UIColor.black.setFill()
            context.fill(CGRect(origin: .zero, size: size))
        }
    }
}
