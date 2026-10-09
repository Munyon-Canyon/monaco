import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var proposalComments: CommentsModel? = nil
}

enum ProposalCommentsSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalCommentsSlotView()
    }
}

private struct ProposalCommentsSlotView: View {
    @Environment(\.proposalComments) private var model
    @Environment(\.sectionScrollProxy) private var scroll

    var body: some View {
        if let model {
            CommentThreadView(model: model)
                .padding(.vertical, MonacoTheme.Space.m)
                .onChange(of: model.lastPostedID) { _, id in
                    guard let id else { return }
                    withAnimation { scroll?.scrollTo(id, anchor: .center) }
                }
        }
    }
}
