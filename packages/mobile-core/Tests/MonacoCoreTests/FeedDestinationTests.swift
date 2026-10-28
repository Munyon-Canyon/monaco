import MonacoAPI
import MonacoCore
import XCTest

final class FeedDestinationTests: XCTestCase {
    private let proposalID = "00000000-0000-7000-8000-0000000fa001"

    func testEveryKindOpensWhereTheRuleSays() {
        let table: [(kind: String, shape: Components.Schemas.FeedItem.ShapePayload?, opens: FeedDestination)] = [
            ("proposal", nil, .proposal(proposalID: proposalID)),
            ("reply", nil, .proposal(proposalID: proposalID)),
            ("result", .cabal, .proposal(proposalID: proposalID)),
            ("result", .member, .post(itemID: "item")),
            ("result", nil, .post(itemID: "item")),
            ("trade", nil, .post(itemID: "item")),
            ("threshold", nil, .post(itemID: "item")),
            ("member_joined", nil, .post(itemID: "item")),
            ("cabal_created", nil, .post(itemID: "item")),
            ("something_new", nil, .post(itemID: "item")),
        ]
        for row in table {
            let item = item(row.kind, shape: row.shape)
            let label = "\(row.kind) \(String(describing: row.shape))"
            XCTAssertEqual(FeedDestination.destination(for: item), row.opens, label)
        }
    }

    func testAProposalWithoutAProposalIDOpensOnItsRefID() {
        XCTAssertEqual(
            FeedDestination.destination(for: item("proposal", proposalID: nil)), .proposal(proposalID: proposalID))
    }

    func testAReplyOrResultWithoutAProposalIDOpensThePost() {
        XCTAssertEqual(FeedDestination.destination(for: item("reply", proposalID: nil)), .post(itemID: "item"))
        XCTAssertEqual(
            FeedDestination.destination(for: item("result", shape: .cabal, proposalID: nil)), .post(itemID: "item"))
    }

    func testEverySampleOpensSomewhere() {
        for sample in Components.Schemas.FeedItem.samples + Components.Schemas.FeedItem.results {
            switch FeedDestination.destination(for: sample) {
            case .proposal(let id): XCTAssertFalse(id.isEmpty)
            case .post(let id): XCTAssertEqual(id, sample.id)
            }
        }
    }

    private func item(
        _ kind: String, shape: Components.Schemas.FeedItem.ShapePayload? = nil,
        proposalID: String? = "00000000-0000-7000-8000-0000000fa001"
    ) -> Components.Schemas.FeedItem {
        Components.Schemas.FeedItem(
            id: "item", kind: kind, refType: "proposals", refId: proposalID ?? "00000000-0000-7000-8000-0000000fa001",
            title: "t", tone: "neutral", proposalId: proposalID, commentCount: 0,
            threadItemId: "item", replyCount: 0, canReply: true, canReact: true, replyRule: .anyone,
            replyRuleCabalName: nil, likeCount: 0, likedByMe: false, reactions: [], reactionCount: 0, shape: shape,
            returnPrivate: false, createdAt: Date(), updatedAt: Date())
    }
}
