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
    @State private var detail = ProposalDetailBox()
    @State private var refresh = ScreenRefresh()

    init(proposalID: String, sections: [any ProposalSection.Type] = Self.sections) {
        self.context = ProposalContext(proposalID: proposalID)
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        if SectionStack<ProposalContext>.live(sections).isEmpty {
            NotMigratedView(screen: "Proposal")
        } else {
            let model = detail.model(proposalID: context.proposalID, environment: environment)
            SectionStack(context: context, sections: sections)
                .environment(\.proposalComments, comments)
                .environment(\.proposalDetail, model)
                .environment(refresh)
                .refreshable { await refresh.run() }
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        MonacoGlassButton(
                            icon: .share, accessibilityLabel: "Share", style: .quiet, placement: .toolbar
                        ) {
                            share(model.summary?.title(asset: model.asset))
                        }
                        .accessibilityIdentifier("proposal-share")
                    }
                }
                .safeAreaInset(edge: .bottom, spacing: 0) {
                    if let comments {
                        CommentComposerBar(
                            model: comments, thesis: model.value?.summary.thesis, offersPoll: true, asksCabal: true)
                    }
                }
                .task {
                    let model = preparedComments()
                    await model.load()
                    await model.observe()
                }
                .onScreenVisibilityChange { comments?.setVisible($0) }
        }
    }

    private func share(_ title: String?) {
        SystemShare.present(SharePayload(title: title ?? "Proposal", url: SharePayload.fallbackURL))
    }

    private func preparedComments() -> CommentsModel {
        if let comments { return comments }
        let created = CommentsModel(
            source: ProposalCommentsSource(proposalID: context.proposalID, api: environment.api),
            hints: environment.hints, clock: ContinuousClock(),
            reactions: ReactionsModel(api: environment.api, clock: ContinuousClock()))
        comments = created
        return created
    }
}
