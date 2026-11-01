import MonacoAPI
import MonacoCore
import XCTest

final class FeedModerationTests: XCTestCase {
    private let tradeByAlex: Components.Schemas.FeedItem = Components.Schemas.FeedItem.samples[1]

    private func reply(by actorID: String?) -> Components.Schemas.FeedItem {
        var item: Components.Schemas.FeedItem = tradeByAlex
        item.kind = "reply"
        item.refType = "feed_comments"
        item.refId = "00000000-0000-7000-8000-00000000c0de"
        item.actorId = actorID
        return item
    }

    func testAPostWithAnActorReportsTheActorAndBlocksThem() throws {
        let actor: String = try XCTUnwrap(tradeByAlex.actorId)
        XCTAssertEqual(
            FeedModeration.reportTarget(for: tradeByAlex, viewerID: nil),
            FeedReportTarget(kind: .user, targetID: actor))
        XCTAssertEqual(
            FeedModeration.blockTarget(for: tradeByAlex, viewerID: nil),
            FeedBlockTarget(userID: actor, name: tradeByAlex.actorName ?? ""))
    }

    func testAReplyReportsTheCommentItCameFrom() throws {
        let actor: String = try XCTUnwrap(tradeByAlex.actorId)
        let item: Components.Schemas.FeedItem = reply(by: actor)
        XCTAssertEqual(
            FeedModeration.reportTarget(for: item, viewerID: nil),
            FeedReportTarget(kind: .comment, targetID: "00000000-0000-7000-8000-00000000c0de"))
    }

    func testTheViewersOwnPostOffersNeitherAction() throws {
        let actor: String = try XCTUnwrap(tradeByAlex.actorId)
        XCTAssertNil(FeedModeration.reportTarget(for: tradeByAlex, viewerID: actor.uppercased()))
        XCTAssertNil(FeedModeration.blockTarget(for: tradeByAlex, viewerID: actor.lowercased()))
    }

    func testASystemItemOffersNeitherAction() {
        let item: Components.Schemas.FeedItem = reply(by: nil)
        XCTAssertNil(FeedModeration.reportTarget(for: item, viewerID: nil))
        XCTAssertNil(FeedModeration.blockTarget(for: item, viewerID: nil))
    }
}
