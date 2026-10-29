import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var proposalComments: CommentsModel? = nil
    @Entry var proposalFocus: ProposalFocus = .none
}

enum ProposalCommentsSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalCommentsSlotView()
    }
}

@MainActor
enum ProposalFocusDriver {
    static let maxPages = 10
    static let deletedMessage = "That reply was deleted."
    static let repliesID = "proposal-replies"

    static func apply(
        _ focus: ProposalFocus, to model: CommentsModel, scroll: (String, UnitPoint) -> Void,
        notify: (String) -> Void
    ) async {
        switch focus {
        case .none:
            return
        case .replies:
            scroll(repliesID, .top)
            if model.canComment { model.presentReplySheet(.thread) }
        case .reply(let id):
            guard let row = await find(id, in: model) else { return notify(deletedMessage) }
            scroll(row.id, .center)
            if model.canComment { model.presentReplySheet(.comment(row)) }
        case .anchor(let id):
            guard let row = await find(id, in: model) else { return notify(deletedMessage) }
            scroll(row.id, .center)
        }
    }

    private static func find(_ id: String, in model: CommentsModel) async -> CommentThreadRow? {
        var pages = 0
        while model.rows.first(where: { $0.id == id }) == nil, model.hasMore, pages < maxPages {
            await model.loadMore()
            pages += 1
        }
        return model.rows.first { $0.id == id }
    }
}

private struct ProposalCommentsSlotView: View {
    @Environment(\.proposalComments) private var model
    @Environment(\.sectionScrollProxy) private var scroll
    @Environment(\.proposalFocus) private var focus
    @Environment(ToastCenter.self) private var toasts
    @State private var appliedFocus = false

    var body: some View {
        if let model {
            CommentThreadView(model: model)
                .id(ProposalFocusDriver.repliesID)
                .onChange(of: model.lastPostedID) { _, id in
                    guard let id else { return }
                    withAnimation { scroll?.scrollTo(id, anchor: .center) }
                }
                .task(id: isReady(model)) { await applyFocus(to: model) }
        }
    }

    private func isReady(_ model: CommentsModel) -> Bool {
        if case .failed = model.phase { return false }
        return model.phase != .loading && (model.canCommentKnown || model.canCommentFailed)
    }

    private func applyFocus(to model: CommentsModel) async {
        guard !appliedFocus, isReady(model) else { return }
        appliedFocus = true
        let scroll = scroll
        let toasts = toasts
        await ProposalFocusDriver.apply(
            focus, to: model,
            scroll: { id, anchor in withAnimation { scroll?.scrollTo(id, anchor: anchor) } },
            notify: { toasts.current = MonacoToast(message: $0) })
    }
}
