import MonacoCore
import SwiftUI

struct ProposalVotesSheet: View {
    let model: ProposalDetailModel?
    @MonacoReduceTransparency private var reduceTransparency

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
        .presentationDetents([.fraction(MonacoSheetDetents.halfFraction)])
        .presentationDragIndicator(.visible)
        .presentationContentInteraction(.scrolls)
        .modifier(ProposalVotesBackground(solid: reduceTransparency))
    }
}

private struct ProposalVotesBackground: ViewModifier {
    let solid: Bool

    @ViewBuilder
    func body(content: Content) -> some View {
        if solid {
            content.presentationBackground(MonacoTheme.canvas)
        } else if #available(iOS 26, *) {
            content
        } else {
            content.presentationBackground(.regularMaterial)
        }
    }
}
