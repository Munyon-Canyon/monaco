import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalCommentsModelTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply
    private let itemID = "item-1"
    private let clock = TestClock()

    func testAPendingProposalItemIsRetriedOnceASecondLater() async throws {
        let pending = try Reply.problem(Support.pendingProblem)
        let transport = StubTransport(scripted: [pending, try Support.page(Thread.samples, feedObjectID: itemID)])
        let model = proposalModel(transport)

        let loading = Task { await model.load() }
        let slept = await Support.waitUntil { self.clock.state.current.pending == 1 }
        XCTAssertTrue(slept)
        XCTAssertEqual(clock.state.current.requested, [CommentsModel.pendingRetryDelay])
        clock.advance(by: CommentsModel.pendingRetryDelay)
        await loading.value

        XCTAssertEqual(model.rows.count, 4)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.prefix(2), ["/v1/proposals/prop-1/comments", "/v1/proposals/prop-1/comments"])
    }

    func testAProposalStillPendingAfterTheRetryFails() async throws {
        let pending = try Reply.problem(Support.pendingProblem)
        let transport = StubTransport(scripted: [pending, pending])
        let model = proposalModel(transport)

        let loading = Task { await model.load() }
        _ = await Support.waitUntil { self.clock.state.current.pending == 1 }
        clock.advance(by: CommentsModel.pendingRetryDelay)
        await loading.value

        guard case .failed(.problem(let problem)) = model.phase else { return XCTFail("\(model.phase)") }
        XCTAssertEqual(problem.code, .known(.feedItemPending))
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testAProposalThreadReadsItsFeedItemForCanComment() async throws {
        let transport = StubTransport(scripted: [
            try Support.page(Thread.samples, feedObjectID: "feed-9"), try Support.detail(canComment: false),
        ])
        let model = proposalModel(transport)

        await model.load()

        XCTAssertFalse(model.canComment)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, ["/v1/proposals/prop-1/comments", "/v1/feed/feed-9"])
    }

    func testAProposalDeleteGoesThroughTheFeedCommentRoute() async throws {
        let mine = Thread(comment: .sample("c1", mine: true), replies: [])
        let transport = StubTransport(scripted: [
            try Support.page([mine], feedObjectID: "feed-9"), try Support.detail(canComment: true),
            .json(.noContent, ""), try Support.page([], feedObjectID: "feed-9"),
        ])
        let model = proposalModel(transport)
        await model.load()

        await model.delete(model.rows[0])

        let sent = await transport.sent
        XCTAssertEqual(sent.first { $0.method == .delete }?.path, "/v1/feed/comments/c1")
    }

    private func proposalModel(_ transport: StubTransport) -> CommentsModel {
        CommentsModel(
            source: ProposalCommentsSource(proposalID: "prop-1", api: Support.api(transport)),
            hints: FakeHintStream(), clock: clock)
    }
}
