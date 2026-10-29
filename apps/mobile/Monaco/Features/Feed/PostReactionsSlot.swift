import MonacoAPI
import MonacoCore
import SwiftUI

enum ReplyControlAction: Equatable {
    case openProposal(proposalID: String, focus: ProposalFocus)
    case presentSheet
    case openPost(itemID: String)

    static func shownCount(for item: Components.Schemas.FeedItem) -> Int {
        item.kind == "reply" ? 0 : Int(item.replyCount)
    }

    static func action(for item: Components.Schemas.FeedItem) -> ReplyControlAction {
        switch FeedDestination.destination(for: item) {
        case .proposal(let proposalID):
            return .openProposal(
                proposalID: proposalID, focus: item.kind == "reply" ? .reply(commentID: item.refId) : .replies)
        case .post(let itemID):
            return item.canReply ? .presentSheet : .openPost(itemID: itemID)
        }
    }
}

private struct ReplySheetState: Identifiable {
    let id = UUID()
    let model: CommentsModel
}

struct PostReactionsSlot: View {
    let item: Components.Schemas.FeedItem
    let model: FeedModel

    @Environment(AppEnvironment.self) private var environment
    @State private var sheet: ReplySheetState?

    var body: some View {
        ReactionRow(
            target: .feedItem(item.id),
            reactions: model.reactions(for: item),
            onToggle: { emoji in Task { await model.react(emoji, on: item) } }
        ) {
            MonacoReactionChip.reply(count: ReplyControlAction.shownCount(for: item), action: reply)
                .accessibilityIdentifier("feed-reply-\(item.id)")
        }
        .sheet(item: $sheet) { state in
            ReplySheet(model: state.model, target: .post(item))
                .onChange(of: state.model.lastPostedID) { _, id in
                    if id != nil { sheet = nil }
                }
        }
    }

    private func reply() {
        switch ReplyControlAction.action(for: item) {
        case .openProposal(let proposalID, let focus):
            environment.navigator.open(ProposalRoute(proposalID: proposalID, focus: focus), in: .feed)
        case .presentSheet:
            let feed = model
            let itemID = item.id
            let comments = CommentsModel.forPost(
                item: item, api: environment.api, hints: environment.hints, clock: ContinuousClock(),
                onPosted: { feed.didReply(to: itemID) })
            comments.presentReplySheet(.post(item))
            sheet = ReplySheetState(model: comments)
        case .openPost(let itemID):
            environment.navigator.open(FeedItemRoute(itemID: itemID), in: .feed)
        }
    }
}
