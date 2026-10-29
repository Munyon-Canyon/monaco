import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct FeedReplyControlTests {
    private typealias Item = Components.Schemas.FeedItem

    private var proposal: Item { Item.samples[0] }
    private var cabalResult: Item { Item.results[0] }
    private var memberSale: Item { Item.results[1] }
    private var noReplies: Item { Item.results[2] }
    private var quote: Item {
        var item = Item.threads[1]
        item.replyCount = 4
        item.proposalId = "proposal-tsla"
        return item
    }

    @Test func theCountIsTheThreadsRepliesOnEveryKindButAReply() {
        #expect(PostReactionsSlot.shownCount(for: proposal) == 6)
        #expect(PostReactionsSlot.shownCount(for: cabalResult) == 5)
        #expect(PostReactionsSlot.shownCount(for: memberSale) == 5)
        #expect(PostReactionsSlot.shownCount(for: Item.samples[4]) == 1)
    }

    @Test func aReplyPostDrawsTheBareBubbleEvenWhenItsThreadHasReplies() {
        #expect(quote.replyCount == 4)
        #expect(PostReactionsSlot.shownCount(for: quote) == 0)
        #expect(PostReactionsSlot.spokenLabel(for: quote) == "Reply")
    }

    @Test func noRepliesHidesTheCount() {
        #expect(noReplies.replyCount == 0)
        #expect(PostReactionsSlot.shownCount(for: noReplies) == 0)
        #expect(PostReactionsSlot.spokenLabel(for: noReplies) == "Reply")
    }

    @Test func theControlReadsReplyAndTheCount() {
        #expect(PostReactionsSlot.spokenLabel(for: proposal) == "Reply, 6 replies")
        #expect(PostReactionsSlot.spokenLabel(for: Item.samples[4]) == "Reply, 1 reply")
    }

    @Test func aProposalOpensItsRepliesWithTheSheetUp() {
        let id = proposal.proposalId ?? proposal.refId
        #expect(
            PostReactionsSlot.action(for: proposal)
                == .openProposal(ProposalRoute(proposalID: id, focus: .replies)))
    }

    @Test func aCabalsResultOpensTheProposalsReplies() throws {
        let id = try #require(cabalResult.proposalId)
        #expect(
            PostReactionsSlot.action(for: cabalResult)
                == .openProposal(ProposalRoute(proposalID: id, focus: .replies)))
    }

    @Test func aReplyPostOpensTheProposalAtThatReply() throws {
        let id = try #require(quote.proposalId)
        #expect(
            PostReactionsSlot.action(for: quote)
                == .openProposal(ProposalRoute(proposalID: id, focus: .reply(commentID: quote.refId))))
    }

    @Test func aMembersSaleOpensTheReplySheetOverTheFeed() {
        #expect(memberSale.canReply)
        #expect(PostReactionsSlot.action(for: memberSale) == .replySheet)
    }

    @Test func aPostTheViewerCannotReplyToOpensThePostPage() {
        var locked = memberSale
        locked.canReply = false
        #expect(PostReactionsSlot.action(for: locked) == .openPost(FeedItemRoute(itemID: locked.id)))
    }

    @Test func theSheetNeverReplacesAProposalsRoute() {
        var locked = proposal
        locked.canReply = false
        if case .openProposal = PostReactionsSlot.action(for: locked) { return }
        Issue.record("a proposal opens its page")
    }
}

@MainActor
struct ProposalFocusDriverTests {
    private typealias Thread = Components.Schemas.CommentThread

    @Test func noFocusDoesNothing() async {
        let (model, run) = await loaded()

        await run(.none)

        #expect(model.replySheet == nil)
        #expect(run.scrolled.isEmpty)
    }

    @Test func repliesScrollsToTheThreadAndOpensTheSheetWithNoQuote() async {
        let (model, run) = await loaded()

        await run(.replies)

        #expect(run.scrolled == [ProposalFocusDriver.repliesID])
        #expect(model.replySheet == .thread)
    }

    @Test func aReplyScrollsToTheCommentAndOpensTheSheetQuotingIt() async {
        let (model, run) = await loaded()

        await run(.reply(commentID: "c2"))

        #expect(run.scrolled == ["c2"])
        if case .comment(let row) = model.replySheet {
            #expect(row.id == "c2")
        } else {
            Issue.record("no quoting sheet")
        }
    }

    @Test func anAnchorScrollsToTheCommentAndOpensNoSheet() async {
        let (model, run) = await loaded()

        await run(.anchor(commentID: "c3"))

        #expect(run.scrolled == ["c3"])
        #expect(model.replySheet == nil)
        #expect(!model.isDockedFocused)
    }

    @Test func aCommentOnALaterPageIsPagedInFirst() async {
        let (model, run) = await loaded(pages: [[.samples[0]], [.samples[1]]])
        #expect(!model.rows.contains { $0.id == "c4" })

        await run(.anchor(commentID: "c4"))

        #expect(run.scrolled == ["c4"])
        #expect(run.notices.isEmpty)
    }

    @Test func aMissingCommentStaysPutAndSaysItWasDeleted() async {
        let (model, run) = await loaded()

        await run(.reply(commentID: "gone"))

        #expect(run.scrolled.isEmpty)
        #expect(model.replySheet == nil)
        #expect(run.notices == ["That reply was deleted."])
    }

    @Test func aMemberWhoCannotCommentGetsNoSheet() async {
        let (model, run) = await loaded(canComment: false)

        await run(.replies)

        #expect(model.replySheet == nil)
    }

    private func loaded(pages: [[Thread]] = [Thread.samples], canComment: Bool = true) async -> (CommentsModel, Run) {
        let model = CommentsModel(
            source: PagedComments(pages: pages, canComment: canComment), hints: FakeHintSource(),
            clock: ContinuousClock())
        await model.load()
        return (model, Run(model))
    }

    @MainActor
    final class Run {
        let model: CommentsModel
        var scrolled: [String] = []
        var notices: [String] = []

        init(_ model: CommentsModel) {
            self.model = model
        }

        func callAsFunction(_ focus: ProposalFocus) async {
            await ProposalFocusDriver.apply(
                focus, to: model, scroll: { id, _ in self.scrolled.append(id) },
                notify: { self.notices.append($0) })
        }
    }
}

private struct PagedComments: CommentsSource {
    let pages: [[Components.Schemas.CommentThread]]
    let canComment: Bool

    func list(cursor: String?) async throws -> CommentsPage {
        let index = cursor.flatMap(Int.init) ?? 0
        let next = index + 1 < pages.count ? String(index + 1) : nil
        return CommentsPage(threads: pages[index], nextCursor: next)
    }

    func post(
        body _: String?, attachment _: Components.Schemas.CommentAttachmentInput?, parentId _: String?,
        submission _: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        throw URLError(.badURL)
    }

    func delete(id _: String, submission _: IdempotentSubmission) async throws {}

    func canComment() async throws -> Bool { canComment }
}
