import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class PushRouteTests: XCTestCase {
    private let user = "0192e8a1-0001-7000-8000-000000000001"
    private let cabal = "0192e8a1-0002-7000-8000-000000000002"
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"
    private let txn = "0192e8a1-0004-7000-8000-000000000004"
    private let feedItem = "0192e8a1-0005-7000-8000-000000000005"
    private let message = "0192e8a1-0006-7000-8000-000000000006"
    private let parent = "0192e8a1-0007-7000-8000-000000000007"
    private let notAnID = "not-a-uuid"

    func testEveryKindTheBackendSendsOpensItsScreen() {
        let rows: [(kind: String, data: [String: Any], route: PushRoute)] = [
            ("new_follower", ["user_id": user], .userProfile(userID: user)),
            ("chat_mention", ["cabal_id": cabal, "message_id": message], .chat(cabalID: cabal)),
            ("chat_thread_reply", ["cabal_id": cabal, "message_id": message], .chat(cabalID: cabal)),
            (
                "chat_thread_reply", ["cabal_id": cabal, "message_id": message, "parent_id": parent],
                .chatThread(cabalID: cabal, parentID: parent)
            ),
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
                "proposal_result", ["cabal_id": cabal, "proposal_id": proposal, "outcome": "filled"],
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

    func testAMentionInAThreadOpensTheThread() {
        let inThread = wire("chat_mention", ["cabal_id": cabal, "message_id": message, "parent_id": parent])
        let inChannel = wire("chat_mention", ["cabal_id": cabal, "message_id": message])

        XCTAssertEqual(PushRoute.parse(inThread), .chatThread(cabalID: cabal, parentID: parent))
        XCTAssertEqual(PushRoute.parse(inChannel), .chat(cabalID: cabal))
    }

    func testAnIDThatIsNotAUUIDSkipsItsRuleAndFallsToALaterOne() {
        let rows: [(kind: String, data: [String: Any], route: PushRoute)] = [
            ("new_follower", ["user_id": notAnID, "cabal_id": cabal], .cabal(cabalID: cabal)),
            ("new_follower", ["user_id": notAnID], .home),
            ("chat_mention", ["cabal_id": notAnID, "message_id": message], .home),
            ("chat_thread_reply", ["cabal_id": cabal, "parent_id": notAnID], .chat(cabalID: cabal)),
            ("chat_mention", ["cabal_id": cabal, "parent_id": notAnID], .chat(cabalID: cabal)),
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

    func testAWeeklyLedgerPushOpensItsWeekAndAnythingElseOpensHome() {
        XCTAssertEqual(PushRoute.parse(wire("weekly_ledger", ["week": "2026-W42"])), .ledger(week: "2026-W42"))
        XCTAssertEqual(
            PushRoute.parse(wire("weekly_ledger", ["week": "2026-W42", "cabal_id": cabal])), .ledger(week: "2026-W42"))
        for bad in ["2026-42", "2026-W4", "26-W42", "2026-W42\n", "x2026-W42", ""] {
            XCTAssertEqual(PushRoute.parse(wire("weekly_ledger", ["week": bad])), .home, "\"\(bad)\"")
        }
        XCTAssertEqual(PushRoute.parse(wire("weekly_ledger", ["week": 42])), .home)
        XCTAssertEqual(PushRoute.parse(wire("weekly_ledger", [:])), .home)
        XCTAssertEqual(PushRoute.parse(wire("nudge", ["week": "2026-W42"])), .home)
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

    private let comment = "0192e8a1-0008-7000-8000-000000000008"

    func testAReplyWithItsCommentOpensTheReplyEvenOnAPost() {
        let post = wire("post_reply", ["feed_item_id": feedItem, "comment_id": comment, "cabal_id": cabal])
        let onProposal = wire(
            "comment_reply",
            ["feed_item_id": feedItem, "comment_id": comment.uppercased(), "proposal_id": proposal, "cabal_id": cabal])

        XCTAssertEqual(
            PushRoute.parse(post),
            .reply(ReplyDestination(feedItemID: feedItem, commentID: comment, proposalID: nil, cabalID: cabal)))
        XCTAssertEqual(
            PushRoute.parse(onProposal),
            .reply(ReplyDestination(feedItemID: feedItem, commentID: comment, proposalID: proposal, cabalID: cabal)))
    }

    func testAReplyWithoutItsCommentKeepsTheOldRoute() {
        XCTAssertEqual(
            PushRoute.parse(
                wire("comment_reply", ["feed_item_id": feedItem, "proposal_id": proposal, "cabal_id": cabal])),
            .proposal(proposalID: proposal, cabalID: cabal))
        XCTAssertEqual(
            PushRoute.parse(wire("comment_reply", ["feed_item_id": feedItem])), .feedItem(feedItemID: feedItem))
        XCTAssertEqual(
            PushRoute.parse(wire("post_reply", ["feed_item_id": feedItem])), .feedItem(feedItemID: feedItem))
        XCTAssertEqual(
            PushRoute.parse(wire("post_reply", ["feed_item_id": feedItem, "comment_id": notAnID])),
            .feedItem(feedItemID: feedItem))
    }

    func testAMentionOpensItsCommentLikeAReply() {
        let onPost = wire("comment_mention", ["feed_item_id": feedItem, "comment_id": comment])
        let onProposal = wire(
            "comment_mention",
            ["feed_item_id": feedItem, "comment_id": comment, "proposal_id": proposal, "cabal_id": cabal])

        XCTAssertEqual(
            PushRoute.parse(onPost),
            .reply(ReplyDestination(feedItemID: feedItem, commentID: comment, proposalID: nil, cabalID: nil)))
        XCTAssertEqual(
            PushRoute.parse(onProposal),
            .reply(ReplyDestination(feedItemID: feedItem, commentID: comment, proposalID: proposal, cabalID: cabal)))
        XCTAssertEqual(
            PushRoute.parse(wire("comment_mention", ["feed_item_id": feedItem, "comment_id": notAnID])),
            .feedItem(feedItemID: feedItem))
        XCTAssertEqual(
            PushRoute.parse(
                wire("comment_mention", ["feed_item_id": feedItem, "proposal_id": proposal, "cabal_id": cabal])),
            .proposal(proposalID: proposal, cabalID: cabal))
    }

    func testAccessPushesOpenTheRequestsAndTheChat() {
        XCTAssertEqual(
            PushRoute.parse(wire("cabal_access_requested", ["cabal_id": cabal])), .joinRequests(cabalID: cabal))
        XCTAssertEqual(PushRoute.parse(wire("cabal_access_approved", ["cabal_id": cabal])), .chat(cabalID: cabal))
        XCTAssertEqual(PushRoute.parse(wire("cabal_access_requested", [:])), .home)
        XCTAssertEqual(PushRoute.parse(wire("cabal_access_approved", ["cabal_id": notAnID])), .home)
    }

    func testACatchUpChatItemOpensItsCabalChatAndOthersDoNot() {
        let at = Date(timeIntervalSince1970: 1_760_000_000)
        let chat = Components.Schemas.CatchUpItem(
            key: "chat:\(message)", kind: .mention, unread: true, at: at,
            cabal: .init(id: cabal.uppercased(), name: "Degens", pictureUrl: nil))
        var noCabal = chat
        noCabal.cabal = nil
        var comment = chat
        comment.key = "comment:\(message)"

        XCTAssertEqual(PushRoute.catchUpChat(chat), .chat(cabalID: cabal))
        XCTAssertNil(PushRoute.catchUpChat(noCabal))
        XCTAssertNil(PushRoute.catchUpChat(comment))
    }

    func testACatchUpReplyMakesTheSameDestinationAsItsPush() throws {
        let at = Date(timeIntervalSince1970: 1_760_000_000)
        let incoming = Components.Schemas.CatchUpComment(
            feedItemId: feedItem.uppercased(), commentId: comment, parentCommentId: nil, rootCommentId: comment,
            proposalId: proposal, cabalId: cabal, author: .init(id: user, name: "Sam", photoUrl: nil), body: "Hi",
            createdAt: at, reactions: [], reactionCount: 0)
        let item = Components.Schemas.CatchUpItem(
            key: "comment:1", kind: .reply, unread: true, at: at, comment: incoming)
        let push = wire(
            "comment_reply",
            ["feed_item_id": feedItem, "comment_id": comment, "proposal_id": proposal, "cabal_id": cabal])

        let destination = try XCTUnwrap(ReplyDestination(catchUpItem: item))

        XCTAssertEqual(destination.route, PushRoute.parse(push))
        XCTAssertNil(
            ReplyDestination(
                catchUpItem: Components.Schemas.CatchUpItem(
                    key: "comment:2", kind: .mention, unread: true, at: at, comment: incoming)))
        XCTAssertNil(
            ReplyDestination(
                catchUpItem: Components.Schemas.CatchUpItem(
                    key: "comment:3", kind: .reply, unread: true, at: at)))
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

    func testEveryFixtureCarriesItsSendersKeysAndOpensItsScreen() throws {
        let senderKeys: Set<String> = [
            "sender_kind", "sender_id", "sender_name", "sender_image_url",
            "group_id", "group_name", "group_image_url",
        ]
        let replyKeys: Set<String> = [
            "kind", "feed_item_id", "cabal_id", "cabal_name", "comment_id", "root_comment_id",
            "item_kind", "item_label", "item_actor_name", "quoted_excerpt",
        ]
        let rows: [(file: String, keys: Set<String>, screen: String)] = [
            ("proposal_result_filled", ["kind", "cabal_id", "proposal_id", "outcome"], "proposal"),
            ("proposal_result_failed", ["kind", "cabal_id", "proposal_id", "outcome"], "proposal"),
            ("proposal_created", ["kind", "cabal_id", "proposal_id"], "proposal"),
            ("cabal_paused", ["kind", "cabal_id"], "cabal"),
            ("proposal_created_grouped", Set(["kind", "cabal_id", "proposal_id"]).union(senderKeys), "proposal"),
            ("cabal_paused_grouped", Set(["kind", "cabal_id"]).union(senderKeys), "cabal"),
            ("vote_reminder", Set(["kind", "cabal_id", "proposal_id"]).union(senderKeys), "proposal"),
            ("vote_deciding", Set(["kind", "cabal_id", "proposal_id"]).union(senderKeys), "proposal"),
            ("close_reminder", Set(["kind", "cabal_id", "proposal_id"]).union(senderKeys), "proposal"),
            ("deposit_credited", ["kind"], "home"),
            ("nudge", ["kind", "user_id"], "home"),
            ("weekly_ledger", ["kind", "week", "sender_kind"], "ledger"),
            ("new_follower", ["kind", "user_id"], "userProfile"),
            ("chat_mention", ["kind", "cabal_id", "message_id"], "chat"),
            ("chat_thread_reply", ["kind", "cabal_id", "message_id"], "chat"),
            ("comment_reply_feed", ["kind", "feed_item_id"], "feedItem"),
            ("comment_reply_proposal", replyKeys.union(senderKeys).union(["proposal_id"]), "reply"),
            (
                "comment_mention", replyKeys.subtracting(["quoted_excerpt"]).union(senderKeys).union(["proposal_id"]),
                "reply"
            ),
            ("post_reply", replyKeys.union(senderKeys.filter { $0.hasPrefix("sender_") }), "reply"),
            ("malformed", ["kind", "cabal_id", "txn_id"], "home"),
        ]
        var directory = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { directory.deleteLastPathComponent() }
        directory = directory.appendingPathComponent("apps/mobile/PushFixtures")

        let files = try FileManager.default.contentsOfDirectory(atPath: directory.path)
            .filter { $0.hasSuffix(".apns") }
            .sorted()
        XCTAssertEqual(files, rows.map { "\($0.file).apns" }.sorted())
        for row in rows {
            let data = try Data(contentsOf: directory.appendingPathComponent("\(row.file).apns"))
            var payload = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any], row.file)
            XCTAssertEqual(
                payload.removeValue(forKey: "Simulator Target Bundle") as? String, "xyz.monacolabs.app", row.file)
            let alert = (payload.removeValue(forKey: "aps") as? [String: Any])?["alert"] as? [String: Any]
            XCTAssertNotNil(alert?["title"] as? String, row.file)
            XCTAssertNotNil(alert?["body"] as? String, row.file)
            XCTAssertEqual(Set(payload.keys), row.keys, row.file)
            XCTAssertEqual(screen(of: PushRoute.parse(payload)), row.screen, row.file)
        }
    }

    private func screen(of route: PushRoute) -> String {
        switch route {
        case .userProfile: "userProfile"
        case .chat, .chatThread: "chat"
        case .transaction: "transaction"
        case .proposal: "proposal"
        case .voteHold: "voteHold"
        case .feedItem: "feedItem"
        case .reply: "reply"
        case .cabal: "cabal"
        case .joinRequests: "joinRequests"
        case .ledger: "ledger"
        case .home: "home"
        }
    }

    private func wire(_ kind: String, _ data: [String: Any]) -> [String: Any] {
        let aps: [String: Any] = ["alert": ["title": "Title", "body": "Body"]]
        return data.merging(["aps": aps, "kind": kind]) { _, standard in standard }
    }
}
