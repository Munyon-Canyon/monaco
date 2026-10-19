#if DEBUG
import MonacoCore
import SwiftUI

struct GalleryShareCardPage: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoSectionHeader("Share card")
                ForEach(Array(cards.enumerated()), id: \.offset) { _, content in
                    ShareCardView(content: content, face: nil)
                        .environment(\.colorScheme, .dark)
                        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
                        .frame(maxWidth: .infinity)
                }
            }
            .padding(MonacoTheme.Space.gutter)
        }
        .monacoCanvas()
    }

    private var cards: [ShareCardContent] {
        [ShareCardSamples.cabal(.gain), ShareCardSamples.me(.loss), ShareCardSamples.cabalShort]
    }
}
#endif
