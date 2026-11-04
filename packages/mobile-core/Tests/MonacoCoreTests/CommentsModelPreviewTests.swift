import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelPreviewTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport

    private func root(_ id: String, minutesAgo: Double) -> Thread {
        Thread(comment: .sample(id, author: "Sam", body: "Root \(id).", minutesAgo: minutesAgo), replies: [])
    }

    private func loaded(_ threads: [Thread], declared: Int? = nil) async throws -> CommentsModel {
        let transport = StubTransport(scripted: [try Support.page(threads), try Support.detail(canComment: true)])
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: "item-1", api: Support.api(transport)),
            hints: FakeHintStream(), clock: TestClock())
        await model.load()
        if let declared { model.declare(count: declared) }
        return model
    }

    func testThePreviewStopsAfterTwoRootsAndTheLinkShowsTheRest() async throws {
        let model = try await loaded([Thread.samples[1], root("r2", minutesAgo: 5), root("r3", minutesAgo: 6)])

        XCTAssertEqual(model.previewEntries(roots: 2).map(\.id), ["c4", "r2"])
        XCTAssertTrue(model.showsAllRepliesLink(roots: 2))
    }

    func testTheLinkHidesWhenEveryReplyIsAlreadyInThePreview() async throws {
        let model = try await loaded([Thread.samples[1], root("r2", minutesAgo: 5)])

        XCTAssertEqual(model.previewEntries(roots: 2).map(\.id), ["c4", "r2"])
        XCTAssertFalse(model.showsAllRepliesLink(roots: 2))
    }

    func testAnswersFoldedUnderAPreviewRootCountAsReachableThroughTheFold() async throws {
        let model = try await loaded([Thread.samples[0], root("r2", minutesAgo: 5)])

        XCTAssertEqual(model.previewEntries(roots: 2).map(\.id), ["c1", "fold-c1", "r2"])
        XCTAssertFalse(model.showsAllRepliesLink(roots: 2))
    }

    func testAnswersFoldedUnderAHiddenRootKeepTheLink() async throws {
        let model = try await loaded([Thread.samples[1], root("r2", minutesAgo: 5), Thread.samples[0]])

        XCTAssertEqual(model.previewEntries(roots: 2).map(\.id), ["c4", "r2"])
        XCTAssertTrue(model.showsAllRepliesLink(roots: 2))
    }

    func testTheLinkShowsWhenTheDeclaredCountExceedsTheLoadedPage() async throws {
        let model = try await loaded([Thread.samples[1], root("r2", minutesAgo: 5)], declared: 6)

        XCTAssertTrue(model.showsAllRepliesLink(roots: 2))
    }
}
