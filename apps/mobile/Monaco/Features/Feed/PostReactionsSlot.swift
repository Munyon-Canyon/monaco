import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedReplyAction: Equatable {
    case openProposal(ProposalRoute)
    case replySheet
    case openPost(FeedItemRoute)
}

struct PostReactionsSlot: View {
    let item: Components.Schemas.FeedItem
    let model: FeedModel
    let onReply: (Components.Schemas.FeedItem) -> Void

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        ReactionRow(
            target: .feedItem(item.id),
            reactions: model.reactions(for: item),
            showsAdd: item.canReact,
            onToggle: { emoji in Task { await model.react(emoji, on: item) } },
            trailing: {
                MonacoReactionChip.reply(count: Self.shownCount(for: item), action: reply)
                    .accessibilityLabel(Self.spokenLabel(for: item))
                    .accessibilityIdentifier("feed-reply-\(item.id)")
            }
        )
    }

    private func reply() {
        Haptics.selection()
        switch Self.action(for: item) {
        case .openProposal(let route): environment.navigator.open(route, in: .feed)
        case .replySheet: onReply(item)
        case .openPost(let route): environment.navigator.open(route, in: .feed)
        }
    }

    static func shownCount(for item: Components.Schemas.FeedItem) -> Int {
        item.kind == "reply" ? 0 : Int(item.replyCount)
    }

    static func spokenLabel(for item: Components.Schemas.FeedItem) -> String {
        switch shownCount(for: item) {
        case 0: "Reply"
        case 1: "Reply, 1 reply"
        case let count: "Reply, \(count) replies"
        }
    }

    static func action(for item: Components.Schemas.FeedItem) -> FeedReplyAction {
        switch FeedDestination.destination(for: item) {
        case .proposal(let proposalID):
            let focus: ProposalFocus = item.kind == "reply" ? .reply(commentID: item.refId) : .replies
            return .openProposal(ProposalRoute(proposalID: proposalID, focus: focus))
        case .post(let itemID):
            return item.canReply ? .replySheet : .openPost(FeedItemRoute(itemID: itemID))
        }
    }
}
