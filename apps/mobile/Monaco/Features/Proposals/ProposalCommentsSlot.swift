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

    private static let end = "proposal-comments-end"

    var body: some View {
        if let model {
            VStack(spacing: 0) {
                CommentThreadView(model: model)
                Color.clear.frame(height: 1).id(Self.end)
            }
            .padding(.vertical, MonacoTheme.Space.m)
            .onChange(of: model.isPosting) { _, posting in
                guard !posting else { return }
                withAnimation { scroll?.scrollTo(Self.end, anchor: .bottom) }
            }
        }
    }
}
