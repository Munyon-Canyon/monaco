import Foundation
import MonacoCore
import XCTest

final class PushRouteTests: XCTestCase {
    private let user = "0192e8a1-0001-7000-8000-000000000001"
    private let cabal = "0192e8a1-0002-7000-8000-000000000002"
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"
    private let txn = "0192e8a1-0004-7000-8000-000000000004"
    private let feedItem = "0192e8a1-0005-7000-8000-000000000005"
    private let message = "0192e8a1-0006-7000-8000-000000000006"
    private let notAnID = "not-a-uuid"

    func testEveryKindTheBackendSendsOpensItsScreen() {
        let rows: [(kind: String, data: [String: Any], route: PushRoute)] = [
            ("new_follower", ["user_id": user], .userProfile(userID: user)),
            ("chat_mention", ["cabal_id": cabal, "message_id": message], .chat(cabalID: cabal)),
            ("chat_thread_reply", ["cabal_id": cabal, "message_id": message], .chat(cabalID: cabal)),
            ("trade_filled", ["cabal_id": cabal, "txn_id": txn], .transaction(txnID: txn, cabalID: cabal)),
            ("trade_failed", ["cabal_id": cabal, "txn_id": txn], .transaction(txnID: txn, cabalID: cabal)),
            (
                "proposal_created", ["cabal_id": cabal, "proposal_id": proposal],
                .proposal(proposalID: proposal, cabalID: cabal)
            ),
            (
                "proposal_passed", ["cabal_id": cabal, "proposal_id": proposal],
                .proposal(proposalID: proposal, cabalID: cabal)
            ),
            (
                "comment_reply", ["feed_item_id": feedItem, "proposal_id": proposal, "cabal_id": cabal],
                .proposal(proposalID: proposal, cabalID: cabal)
            ),
            ("comment_reply", ["feed_item_id": feedItem], .feedItem(feedItemID: feedItem)),
            ("cabal_paused", ["cabal_id": cabal], .cabal(cabalID: cabal)),
            ("cabal_resumed", ["cabal_id": cabal], .cabal(cabalID: cabal)),
            ("nudge", ["user_id": user], .home),
            ("deposit_credited", [:], .home),
            ("follow_digest", [:], .home),
            ("test", ["user_id": user], .home),
            ("a_kind_from_the_future", [:], .home),
        ]

        for row in rows {
            XCTAssertEqual(PushRoute.parse(wire(row.kind, row.data)), row.route, row.kind)
        }
    }

    func testTheFirstMatchingRuleWins() {
        let rows: [(label: String, kind: String, data: [String: Any], route: PushRoute)] = [
            (
                "a reply on a proposal opens the proposal, not the feed item", "comment_reply",
                ["feed_item_id": feedItem, "proposal_id": proposal, "cabal_id": cabal],
                .proposal(proposalID: proposal, cabalID: cabal)
            ),
            (
                "a new follower with a stray cabal id still opens the profile", "new_follower",
                ["user_id": user, "cabal_id": cabal], .userProfile(userID: user)
            ),
            (
                "a chat push outranks the ids a transaction carries", "chat_mention",
                ["cabal_id": cabal, "txn_id": txn, "proposal_id": proposal], .chat(cabalID: cabal)
            ),
            (
                "a transaction outranks a proposal", "trade_filled",
                ["cabal_id": cabal, "txn_id": txn, "proposal_id": proposal],
                .transaction(txnID: txn, cabalID: cabal)
            ),
            (
                "a proposal outranks the bare cabal", "proposal_created",
                ["cabal_id": cabal, "proposal_id": proposal, "feed_item_id": feedItem],
                .proposal(proposalID: proposal, cabalID: cabal)
            ),
        ]

        for row in rows {
            XCTAssertEqual(PushRoute.parse(wire(row.kind, row.data)), row.route, row.label)
        }
    }

    func testAnIDThatIsNotAUUIDSkipsItsRuleAndFallsToALaterOne() {
        let rows: [(kind: String, data: [String: Any], route: PushRoute)] = [
            ("new_follower", ["user_id": notAnID, "cabal_id": cabal], .cabal(cabalID: cabal)),
            ("new_follower", ["user_id": notAnID], .home),
            ("chat_mention", ["cabal_id": notAnID, "message_id": message], .home),
            ("trade_filled", ["cabal_id": cabal, "txn_id": notAnID], .cabal(cabalID: cabal)),
            ("trade_filled", ["cabal_id": notAnID, "txn_id": txn], .home),
            ("proposal_created", ["cabal_id": cabal, "proposal_id": notAnID], .cabal(cabalID: cabal)),
            ("proposal_created", ["cabal_id": notAnID, "proposal_id": proposal], .home),
            ("comment_reply", ["feed_item_id": notAnID], .home),
            ("cabal_paused", ["cabal_id": notAnID], .home),
            ("cabal_paused", ["cabal_id": ""], .home),
        ]

        for row in rows {
            XCTAssertEqual(PushRoute.parse(wire(row.kind, row.data)), row.route, "\(row.kind) \(row.data)")
        }
    }

    func testOnlyAHyphenatedHexUUIDCountsAsAnID() {
        let forms = [
            "0192e8a1-0002-7000-8000-00000000000g",
            "0192e8a1-0002-7000-8000-0000000000020",
            String(cabal.dropLast()),
            " \(cabal)",
            "\(cabal) ",
            "{\(cabal)}",
        ]

        for form in forms {
            XCTAssertEqual(PushRoute.parse(wire("cabal_paused", ["cabal_id": form])), .home, "\"\(form)\"")
        }
        XCTAssertEqual(PushRoute.parse(wire("cabal_paused", ["cabal_id": cabal])), .cabal(cabalID: cabal))
    }

    func testACabalIDOpensTheCabalOnlyWhenNoOtherUsableIDComesWithIt() {
        let rows: [(kind: String, data: [String: Any], route: PushRoute)] = [
            ("future_kind", ["cabal_id": cabal], .cabal(cabalID: cabal)),
            ("future_kind", ["cabal_id": cabal, "message_id": message], .cabal(cabalID: cabal)),
            ("future_kind", ["cabal_id": cabal, "user_id": notAnID], .cabal(cabalID: cabal)),
            ("future_kind", ["cabal_id": cabal, "user_id": user], .home),
            ("nudge", ["cabal_id": cabal, "user_id": user], .home),
            ("future_kind", ["cabal_id": cabal, "feed_item_id": feedItem], .home),
        ]

        for row in rows {
            XCTAssertEqual(PushRoute.parse(wire(row.kind, row.data)), row.route, "\(row.kind) \(row.data)")
        }
    }

    func testAProposalIDThatIsNotAUUIDDoesNotKeepACommentReplyFromItsFeedItem() {
        let reply = wire("comment_reply", ["feed_item_id": feedItem, "proposal_id": notAnID, "cabal_id": cabal])

        XCTAssertEqual(PushRoute.parse(reply), .feedItem(feedItemID: feedItem))
    }

    func testAProposalReplyWithoutItsCabalIsMalformedAndGoesHome() {
        let reply = wire("comment_reply", ["feed_item_id": feedItem, "proposal_id": proposal])

        XCTAssertEqual(PushRoute.parse(reply), .home)
    }

    func testAPayloadWithoutAKindStillRoutesByItsIDs() {
        XCTAssertEqual(
            PushRoute.parse(["cabal_id": cabal, "txn_id": txn]), .transaction(txnID: txn, cabalID: cabal))
        XCTAssertEqual(
            PushRoute.parse(["cabal_id": cabal, "proposal_id": proposal]),
            .proposal(proposalID: proposal, cabalID: cabal))
        XCTAssertEqual(PushRoute.parse(["cabal_id": cabal]), .cabal(cabalID: cabal))
    }

    func testTheKindsThatNeedTheirOwnKindDoNotMatchWithoutIt() {
        XCTAssertEqual(PushRoute.parse(["user_id": user]), .home)
        XCTAssertEqual(PushRoute.parse(["feed_item_id": feedItem]), .home)
        XCTAssertEqual(PushRoute.parse(["feed_item_id": feedItem, "cabal_id": cabal]), .home)
        XCTAssertEqual(PushRoute.parse([:]), .home)
        XCTAssertEqual(PushRoute.parse(["aps": ["alert": "Hello"]]), .home)
    }

    func testAValueThatIsNotAStringCountsAsMissing() {
        let rows: [(kind: Any, data: [String: Any], route: PushRoute)] = [
            ("trade_filled", ["cabal_id": 42, "txn_id": txn], .home),
            ("trade_filled", ["cabal_id": cabal, "txn_id": NSNull()], .cabal(cabalID: cabal)),
            ("cabal_paused", ["cabal_id": [cabal]], .home),
            ("cabal_paused", ["cabal_id": ["id": cabal]], .home),
            ("new_follower", ["user_id": true], .home),
            ("new_follower", ["user_id": NSNumber(value: 7), "cabal_id": cabal], .cabal(cabalID: cabal)),
            (7, ["cabal_id": cabal], .cabal(cabalID: cabal)),
            (["new_follower"], ["user_id": user], .home),
            (NSNull(), ["cabal_id": cabal, "txn_id": txn], .transaction(txnID: txn, cabalID: cabal)),
        ]

        for row in rows {
            var payload = row.data
            payload["kind"] = row.kind
            XCTAssertEqual(PushRoute.parse(payload), row.route, "\(row.kind) \(row.data)")
        }
    }

    func testIDsComeBackLowercase() {
        let loud = wire("trade_filled", ["cabal_id": cabal.uppercased(), "txn_id": txn.uppercased()])

        XCTAssertEqual(PushRoute.parse(loud), .transaction(txnID: txn, cabalID: cabal))
        XCTAssertNotEqual(cabal, cabal.uppercased())
    }

    private func wire(_ kind: String, _ data: [String: Any]) -> [String: Any] {
        let aps: [String: Any] = ["alert": ["title": "Title", "body": "Body"]]
        return data.merging(["aps": aps, "kind": kind]) { _, standard in standard }
    }
}
