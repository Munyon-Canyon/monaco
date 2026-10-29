import MonacoAPI
import SwiftUI

nonisolated enum GifBubbleMetrics {
    static let width: CGFloat = 200
    static let maxHeight: CGFloat = 260
    static let radius: CGFloat = 16

    static func size(mediaWidth: CGFloat, mediaHeight: CGFloat, available: CGFloat) -> CGSize {
        let width = min(Self.width, max(available, 0))
        let ratio = mediaHeight / max(mediaWidth, 1)
        return CGSize(width: width, height: min(width * ratio, maxHeight))
    }
}

private struct GifBubbleLayout: Layout {
    let mediaWidth: CGFloat
    let mediaHeight: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews _: Subviews, cache _: inout ()) -> CGSize {
        GifBubbleMetrics.size(
            mediaWidth: mediaWidth, mediaHeight: mediaHeight, available: proposal.width ?? GifBubbleMetrics.width)
    }

    func placeSubviews(in bounds: CGRect, proposal _: ProposedViewSize, subviews: Subviews, cache _: inout ()) {
        for subview in subviews {
            subview.place(at: bounds.origin, proposal: ProposedViewSize(bounds.size))
        }
    }
}

struct GifBubble: View {
    let commentID: String
    let attachment: Components.Schemas.GifAttachment

    private let motion = GifMotion()
    @State private var isViewing = false
    @State private var runs = 0

    var body: some View {
        if let still = URL(string: attachment.still.url), let preview = URL(string: attachment.preview.url),
            let original = URL(string: attachment.original.url)
        {
            bubble(still: still, preview: preview)
                .fullScreenCover(isPresented: $isViewing) { MediaViewer(gif: original, still: still) }
        }
    }

    private func bubble(still: URL, preview: URL) -> some View {
        let shape = RoundedRectangle(cornerRadius: GifBubbleMetrics.radius, style: .continuous)
        return GifBubbleLayout(
            mediaWidth: CGFloat(attachment.preview.width), mediaHeight: CGFloat(attachment.preview.height)
        ) {
            ZStack(alignment: .bottomLeading) {
                MonacoTheme.surfaceSunken
                GifImageView(
                    id: commentID, still: still, animated: preview, showsPlay: false, sharedRuns: $runs)
                badge
            }
        }
        .clipShape(shape)
        .contentShape(shape)
        .onTapGesture { isViewing = true }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("GIF: \(attachment.title)")
        .accessibilityAddTraits(.isButton)
        .accessibilityAction { isViewing = true }
        .accessibilityIdentifier("gif-bubble-\(commentID)")
        .overlay {
            if motion.isReduced && runs == 0 {
                GifPlayButton(id: commentID) { runs += 1 }
            }
        }
    }

    private var badge: some View {
        Text("GIF")
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(MonacoTheme.onCover)
            .padding(.horizontal, MonacoTheme.Space.s)
            .padding(.vertical, MonacoTheme.Space.xs)
            .background(MonacoTheme.scrim, in: Capsule())
            .padding(MonacoTheme.Space.s)
            .allowsHitTesting(false)
    }
}
