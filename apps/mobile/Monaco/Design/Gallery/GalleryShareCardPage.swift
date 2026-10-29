#if DEBUG
import MonacoCore
import SwiftUI

struct GalleryShareCardPage: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoSectionHeader("Share card")
                ForEach(Array(cards.enumerated()), id: \.offset) { _, spec in
                    FShareCard(spec: spec, size: CGSize(width: 360, height: 450))
                        .environment(\.colorScheme, .dark)
                        .frame(maxWidth: .infinity)
                }
            }
            .padding(MonacoTheme.Space.gutter)
        }
        .monacoCanvas()
    }

    private var cards: [ShareCardSpec] {
        [ShareSampleData.stake(), ShareSampleData.portfolio()]
    }
}
#endif
