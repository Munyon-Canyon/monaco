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
    @Environment(\.proposalDetail) private var detail
    @Environment(\.proposalFocus) private var focus
    @Environment(\.sectionScrollProxy) private var scroll
    @Environment(ToastCenter.self) private var toasts
    @State private var handled = false

    var body: some View {
        if let model {
            CommentThreadView(model: model)
                .id(ProposalFocus.repliesAnchorID)
                .onChange(of: detail?.value?.replyCount, initial: true) { _, count in
                    if let count { model.declare(count: count) }
                }
                .onChange(of: proposerFirstName, initial: true) { _, name in
                    model.placeholder = name.map { "Reply to \($0)" } ?? CommentsCopy.placeholder
                }
                .onAppear { model.onCountChange = { detail?.setReplyCount($0) } }
                .onChange(of: model.lastPostedID) { _, id in
                    guard let id else { return }
                    Task { await scrollToPosted(id) }
                }
                .task(id: model.phase) { await applyFocus(model) }
        }
    }

    private var proposerFirstName: String? {
        guard let proposerID = detail?.value?.summary.proposerID else { return nil }
        return ProposalHero.firstName(of: detail?.members.first { $0.id == proposerID }?.name)
    }

    private func scrollToPosted(_ id: String) async {
        scroll?.scrollTo(ProposalFocus.repliesAnchorID, anchor: .bottom)
        await Task.yield()
        withAnimation { scroll?.scrollTo(id, anchor: .center) }
    }

    private func applyFocus(_ model: CommentsModel) async {
        guard !handled, focus != .none, model.phase != .loading else { return }
        handled = true
        var pages = 0
        while focus.resolve(in: model.rows, hasMore: model.hasMore) == .needsMore, pages < ProposalFocus.maxPages {
            await model.loadMore()
            pages += 1
        }
        switch focus.resolve(in: model.rows, hasMore: false) {
        case .nothing, .needsMore: break
        case .replySheet:
            scroll?.scrollTo(ProposalFocus.repliesAnchorID, anchor: .top)
            model.presentReplySheet(.thread)
        case .replySheetQuoting(let row):
            scroll?.scrollTo(row.id, anchor: .center)
            model.presentReplySheet(.comment(row))
        case .scroll(let id):
            withAnimation { scroll?.scrollTo(id, anchor: .center) }
        case .notFound:
            toasts.show(success: "That reply was deleted.")
        }
    }
}
