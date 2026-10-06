import MonacoCore
import SwiftUI

struct ProposalScreen: View {
    static let sections: [any ProposalSection.Type] = [
        ProposalDetailSlot.self,
        ProposalCommentsSlot.self,
    ]

    let context: ProposalContext
    let sections: [any ProposalSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @State private var comments: CommentsModel?

    init(proposalID: String, sections: [any ProposalSection.Type] = Self.sections) {
        self.context = ProposalContext(proposalID: proposalID)
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        if SectionStack<ProposalContext>.live(sections).isEmpty {
            NotMigratedView(screen: "Proposal")
        } else {
            SectionStack(context: context, sections: sections)
                .environment(\.proposalComments, comments)
                .safeAreaInset(edge: .bottom, spacing: 0) {
                    if let comments { CommentComposerBar(model: comments) }
                }
                .task {
                    let model = preparedComments()
                    await model.load()
                    await model.observe()
                }
                .onScreenVisibilityChange { comments?.setVisible($0) }
        }
    }

    private func preparedComments() -> CommentsModel {
        if let comments { return comments }
        let created = CommentsModel(
            source: ProposalCommentsSource(proposalID: context.proposalID, api: environment.api),
            hints: environment.hints, clock: ContinuousClock())
        comments = created
        return created
    }
}
