import SwiftUI

struct MediaViewer: View {
    let gif: URL
    let still: URL

    @Environment(\.dismiss) private var dismiss
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var drag: CGFloat = 0

    private static let dismissDistance: CGFloat = 120

    var body: some View {
        ZStack(alignment: .topTrailing) {
            Color.black.ignoresSafeArea()
            GifImageView(id: "viewer", still: still, animated: gif, fills: false)
                .offset(y: drag)
                .gesture(swipeDown)
            MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .onCover) {
                dismiss()
            }
            .accessibilityIdentifier("media-viewer-close")
            .padding(MonacoTheme.Space.s)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("media-viewer")
    }

    private var swipeDown: some Gesture {
        DragGesture()
            .onChanged { drag = max(0, $0.translation.height) }
            .onEnded { value in
                if value.translation.height > Self.dismissDistance {
                    dismiss()
                } else {
                    withAnimation(reduceMotion ? nil : .snappy) { drag = 0 }
                }
            }
    }
}
