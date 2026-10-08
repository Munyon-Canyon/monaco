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
                SectionStackLayout(spacing: SectionStackMetrics.spacing) {
                    ForEach(Array(Self.live(sections).enumerated()), id: \.offset) { _, section in
                        AnyView(section.body(for: context))
                    }
                }
            }
            .environment(\.sectionScrollProxy, proxy)
        }
    }
}

struct SectionStackLayout: Layout {
    let spacing: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = ProposedViewSize(width: proposal.width, height: nil)
        var size = CGSize.zero
        for subview in subviews {
            let fit = subview.sizeThatFits(width)
            guard fit.height > 0 else { continue }
            size.height += (size.height > 0 ? spacing : 0) + fit.height
            size.width = max(size.width, fit.width)
        }
        return size
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let width = ProposedViewSize(width: bounds.width, height: nil)
        var y = bounds.minY
        for subview in subviews {
            let height = subview.sizeThatFits(width).height
            subview.place(at: CGPoint(x: bounds.midX, y: y), anchor: .top, proposal: width)
            if height > 0 {
                y += height + spacing
            }
        }
    }
}
