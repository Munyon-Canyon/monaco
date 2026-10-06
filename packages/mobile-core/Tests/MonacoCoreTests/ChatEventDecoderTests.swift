import Foundation
import MonacoCore
import XCTest

final class ChatEventDecoderTests: XCTestCase {
    func testMessageCreatedDecodesTheStoredMessage() throws {
        let message = ChatFixtures.message("m1", body: "gm")
        let data = Data(try ChatFixtures.json(message).utf8)

        XCTAssertEqual(ChatEventDecoder.decode(name: "message.created", data: data), .messageCreated(message))
    }

    func testMessageCreatedReadsMicrosecondTimestampsFromTheBackend() throws {
        let data = Data(
            #"""
            {"id":"m1","author":{"id":"u1","handle":null,"display_name":"Kai","photo_url":null},
             "body":"gm","created_at":"2026-10-03T12:00:00.123456Z","parent_id":null,"also_in_channel":false,
             "reply_count":0,"last_reply_at":null,"proposal_id":null,"deleted":false}
            """#.utf8)

        guard case .messageCreated(let message) = ChatEventDecoder.decode(name: "message.created", data: data) else {
            return XCTFail("expected a message")
        }
        XCTAssertEqual(message.createdAt.timeIntervalSince1970, 1_791_028_800.123, accuracy: 0.001)
    }

    func testThreadUpdatedCarriesTheNewReplyCount() {
        let data = Data(
            #"{"message_id":"m1","reply_count":3,"last_reply_at":"2026-10-03T12:00:00Z"}"#.utf8)

        XCTAssertEqual(
            ChatEventDecoder.decode(name: "thread.updated", data: data),
            .threadUpdated(id: "m1", replyCount: 3, lastReplyAt: Date(timeIntervalSince1970: 1_791_028_800))
        )
    }

    func testSeenUpdatedCarriesTheCount() {
        let data = Data(#"{"message_id":"m1","count":4}"#.utf8)

        XCTAssertEqual(
            ChatEventDecoder.decode(name: "seen.updated", data: data), .seenUpdated(messageId: "m1", count: 4))
    }

    func testMessageDeletedCarriesTheID() {
        let data = Data(#"{"id":"m1"}"#.utf8)

        XCTAssertEqual(ChatEventDecoder.decode(name: "message.deleted", data: data), .messageDeleted(id: "m1"))
    }

    func testAnUnknownNameIsNotAnEvent() {
        XCTAssertNil(ChatEventDecoder.decode(name: "typing.started", data: Data(#"{"id":"m1"}"#.utf8)))
    }

    func testAKnownNameWithAnUnreadablePayloadIsNotAnEvent() {
        for name in ["message.created", "thread.updated", "seen.updated", "message.deleted"] {
            XCTAssertNil(ChatEventDecoder.decode(name: name, data: Data(#"{"unexpected":true}"#.utf8)), name)
            XCTAssertNil(ChatEventDecoder.decode(name: name, data: Data("not json".utf8)), name)
        }
    }

    func testAnUnreadableTimestampIsNotAnEvent() {
        let data = Data(#"{"message_id":"m1","reply_count":3,"last_reply_at":"garbage"}"#.utf8)

        XCTAssertNil(ChatEventDecoder.decode(name: "thread.updated", data: data))
    }
}
