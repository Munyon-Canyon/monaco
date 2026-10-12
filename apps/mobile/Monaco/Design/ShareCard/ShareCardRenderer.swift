import MonacoCore
import SwiftUI

@MainActor
enum ShareCardRenderer {
    static let scale: CGFloat = 3

    static func render(_ content: ShareCardContent, face: UIImage?) -> UIImage? {
        let card = ShareCardView(content: content, face: face)
            .environment(\.colorScheme, .dark)
            .dynamicTypeSize(.large)
        let renderer = ImageRenderer(content: card)
        renderer.proposedSize = ProposedViewSize(ShareCardView.size)
        renderer.scale = scale
        renderer.isOpaque = true
        return renderer.uiImage
    }

    static func face(for url: URL?) async -> UIImage? {
        guard let url else { return nil }
        return await MonacoRemoteImageStore.avatars.image(for: url)
    }
}
