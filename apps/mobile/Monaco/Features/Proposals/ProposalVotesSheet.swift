import MonacoCore
import SwiftUI

struct ProposalVotesSheet: View {
    let model: ProposalDetailModel?
    @State private var selection: PresentationDetent
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    init(model: ProposalDetailModel?, detent: PresentationDetent = .medium) {
        self.model = model
        _selection = State(initialValue: detent)
    }

    private var detents: [PresentationDetent] {
        dynamicTypeSize.isAccessibilitySize ? [.large] : [.medium, .large]
    }

    private var current: Binding<PresentationDetent> {
        Binding(
            get: { detents.contains(selection) ? selection : .large },
            set: { selection = $0 }
        )
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                MonacoSheetHeading(title: "Votes", identifier: nil, showsClose: true)
                ProposalVotersView(model: model)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .toolbar(.hidden, for: .navigationBar)
            .tabRouteDestinations()
        }
        .presentationDetents(Set(detents), selection: current)
        .presentationDragIndicator(.visible)
        .presentationContentInteraction(.scrolls)
        .modifier(ProposalVotesBackground(isLarge: current.wrappedValue == .large, solid: reduceTransparency))
    }
}

private struct ProposalVotesBackground: ViewModifier {
    let isLarge: Bool
    let solid: Bool

    @ViewBuilder
    func body(content: Content) -> some View {
        if solid {
            content.presentationBackground(MonacoTheme.canvas)
        } else if #available(iOS 26, *) {
            content
        } else {
            content.presentationBackground(
                isLarge ? AnyShapeStyle(MonacoTheme.canvas) : AnyShapeStyle(.regularMaterial))
        }
    }
}
