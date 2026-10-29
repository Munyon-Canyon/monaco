import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelRepliesTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply
    private let itemID = "item-1"
    private let clock = TestClock()

    func testExpandAndCollapseAFoldShowsAndHidesItsAnswers() async throws {
        let (model, _, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])
        await model.load()
        XCTAssertEqual(model.visibleRows.map(\.id), ["c1", "c4"])

        model.expand(rootID: "c1")
        XCTAssertEqual(model.visibleRows.map(\.id), ["c1", "c2", "c3", "c4"])
        XCTAssertTrue(model.isExpanded(rootID: "c1"))

        model.collapse(rootID: "c1")
        XCTAssertEqual(model.visibleRows.map(\.id), ["c1", "c4"])
    }

    func testEntriesPutTheFoldUnderItsRootThenAfterTheAnswersOnceOpen() async throws {
        let (model, _, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])
        await model.load()

        XCTAssertEqual(
            model.entries.map(\.id), ["c1", "fold-c1", "c4"])
        XCTAssertEqual(model.entries[1], .fold(rootID: "c1", count: 2, isExpanded: false))

        model.expand(rootID: "c1")

        XCTAssertEqual(model.entries.map(\.id), ["c1", "c2", "c3", "fold-c1", "c4"])
        XCTAssertEqual(model.entries[3], .fold(rootID: "c1", count: 2, isExpanded: true))
    }

    func testJumpingToAFoldedAnswerExpandsItsRootAndScrollsAndHighlights() async throws {
        let (model, _, _) = try make([
            try Support.page(Thread.loganAAPLThread), try Support.detail(canComment: true),
        ])
        await model.load()
        let christelle = try XCTUnwrap(model.rows.first { $0.id == "aapl-christelle" })
        let quote = try XCTUnwrap(christelle.quote)
        XCTAssertFalse(model.isExpanded(rootID: "aapl-nora"))

        model.jump(to: quote)

        XCTAssertTrue(model.isExpanded(rootID: "aapl-nora"))
        XCTAssertTrue(model.visibleRows.contains { $0.id == "aapl-logan" })
        XCTAssertEqual(model.scrollRequest?.id, "aapl-logan")
        XCTAssertEqual(model.highlightedID, "aapl-logan")
    }

    func testTheHighlightFadesAfterTwelveHundredMilliseconds() async throws {
        let (model, _, _) = try make([
            try Support.page(Thread.loganAAPLThread), try Support.detail(canComment: true),
        ])
        await model.load()
        let quote = try XCTUnwrap(model.rows.first { $0.id == "aapl-sam" }?.quote)

        model.jump(to: quote)
        for _ in 0..<10 { await Task.yield() }
        XCTAssertEqual(model.highlightedID, "aapl-andres")
        clock.advance(by: .milliseconds(1100))
        XCTAssertEqual(model.highlightedID, "aapl-andres")
        clock.advance(by: .milliseconds(200))
        let faded = await Support.waitUntil { model.highlightedID == nil }

        XCTAssertTrue(faded)
    }

    func testAQuoteOfADeletedReplyDoesNotJump() async throws {
        let (model, _, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])
        await model.load()
        let deleted = CommentQuoteRow(
            .sample("c2", root: "c1", author: "Alex", excerpt: nil, folded: true, deleted: true))

        model.jump(to: deleted)

        XCTAssertNil(model.scrollRequest)
        XCTAssertNil(model.highlightedID)
        XCTAssertFalse(model.isExpanded(rootID: "c1"))
    }

    func testAQuoteOfAReplyThatIsNotLoadedDoesNotJump() async throws {
        let (model, _, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])
        await model.load()

        model.jump(to: CommentQuoteRow(.sample("elsewhere", author: "Alex", excerpt: "Hi")))

        XCTAssertNil(model.scrollRequest)
    }

    func testReactingShowsAtOnceAndKeepsTheServersSummaryOnTheRow() async throws {
        let (model, transport, _) = try makeReacting(
            Thread.loganAAPLThread, [try reactionSummary([("❤️", 3, true)], total: 3)])
        await model.load()
        let andres = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })

        await model.react("❤️", to: andres)

        let after = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })
        XCTAssertEqual(after.reactions, [.init(emoji: "❤️", count: 3, mine: true)])
        XCTAssertEqual(after.comment.reactionCount, 3)
        XCTAssertEqual(model.reactions(for: after), after.reactions)
        let sent = await transport.sent.filter { $0.method == .put }
        XCTAssertEqual(sent.first?.path, "/v1/feed/comments/aapl-andres/reactions/%E2%9D%A4%EF%B8%8F")
    }

    func testAFailedReactionRollsBackAndKeepsTheRowAsItWas() async throws {
        let (model, _, _) = try makeReacting(
            Thread.loganAAPLThread, [.failure(URLError(.notConnectedToInternet))])
        await model.load()
        let andres = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })

        await model.react("❤️", to: andres)

        let after = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })
        XCTAssertEqual(after.reactions, [.init(emoji: "❤️", count: 2, mine: false)])
        XCTAssertEqual(model.reactionNotice, "You're offline. Try again.")
        XCTAssertEqual(model.reactionNoticeTick, 1)
    }

    func testDoubleTapAddsAHeartOnceAndDoesNothingWhenItIsAlreadyYours() async throws {
        let (model, transport, _) = try makeReacting(
            Thread.loganAAPLThread, [try reactionSummary([("❤️", 3, true)], total: 3)])
        await model.load()
        let andres = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })

        await model.doubleTap(andres)
        let again = try XCTUnwrap(model.rows.first { $0.id == "aapl-andres" })
        await model.doubleTap(again)

        let sent = await transport.sent.filter { $0.path?.contains("/reactions/") == true }
        XCTAssertEqual(sent.map(\.method.rawValue), ["PUT"])
    }

    func testASystemReplyTakesNoReaction() async throws {
        let (model, transport, _) = try makeReacting(Thread.nvdaPassedMonacoReply.asList, [])
        await model.load()
        let monaco = try XCTUnwrap(model.rows.first)

        await model.react("❤️", to: monaco)
        await model.doubleTap(monaco)

        let sent = await transport.sent.filter { $0.path?.contains("/reactions/") == true }
        XCTAssertTrue(sent.isEmpty)
    }

    func testPostingAnAnswerOpensItsFoldSoTheAuthorSeesIt() async throws {
        let created = Components.Schemas.Comment.sample("c9", author: "Maya", parent: "c1", body: "Thanks.", mine: true)
        let (model, _, _) = try make([
            try Support.page(Thread.samples), try Support.detail(canComment: true),
            .json(.created, try Support.encode(created)), try Support.page(Thread.samples),
        ])
        await model.load()
        model.presentReplySheet(.comment(try XCTUnwrap(model.rows.first { $0.id == "c1" })))

        _ = await model.post(CommentDraft(text: "Thanks."))

        XCTAssertTrue(model.isExpanded(rootID: "c1"))
    }

    private func make(_ script: [Reply]) throws -> (CommentsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: itemID, api: Support.api(transport)), hints: hints, clock: clock)
        return (model, transport, hints)
    }

    private func reactionSummary(_ rows: [(String, Int32, Bool)], total: Int32) throws -> Reply {
        let reactions = rows.map { Components.Schemas.Reaction(emoji: $0.0, count: $0.1, mine: $0.2) }
        return .json(
            .ok, try Support.encode(Components.Schemas.ReactionSummary(reactions: reactions, total: total)))
    }

    private func makeReacting(
        _ threads: [Thread], _ reactionReplies: [Reply]
    ) throws -> (CommentsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(
            scripted: [try Support.page(threads), try Support.detail(canComment: true)] + reactionReplies)
        let hints = FakeHintStream()
        let api = Support.api(transport)
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: itemID, api: api), hints: hints, clock: clock,
            reactions: ReactionsModel(api: api, clock: clock))
        return (model, transport, hints)
    }
}

extension Components.Schemas.CommentThread {
    fileprivate var asList: [Self] { [self] }
}
