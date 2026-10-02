import SwiftUI

struct SectionStack<Context>: View {
    let context: Context
    let sections: [any ScreenSection<Context>.Type]

    static func live(_ sections: [any ScreenSection<Context>.Type]) -> [any ScreenSection<Context>.Type] {
        sections.filter { $0.isLive }
    }

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.gutter) {
                ForEach(Array(Self.live(sections).enumerated()), id: \.offset) { _, section in
                    AnyView(section.body(for: context))
                }
            }
        }
    }
}
