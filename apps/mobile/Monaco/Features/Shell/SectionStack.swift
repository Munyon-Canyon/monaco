import SwiftUI

extension EnvironmentValues {
    @Entry var sectionScrollProxy: ScrollViewProxy? = nil
}

enum SectionStackMetrics {
    static let spacing = MonacoTheme.Space.l
}

struct SectionStack<Context>: View {
    let context: Context
    let sections: [any ScreenSection<Context>.Type]

    static func live(_ sections: [any ScreenSection<Context>.Type]) -> [any ScreenSection<Context>.Type] {
        sections.filter { $0.isLive }
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(spacing: SectionStackMetrics.spacing) {
                    ForEach(Array(Self.live(sections).enumerated()), id: \.offset) { _, section in
                        AnyView(section.body(for: context))
                    }
                }
            }
            .environment(\.sectionScrollProxy, proxy)
            .monacoCanvas()
        }
    }
}
