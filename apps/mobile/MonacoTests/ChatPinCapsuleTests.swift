import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct ChatPinCapsuleTests {
    private let viewer = "u-me"

    private func message(
        _ id: String = "m1", author: String = "u-sam", name: String = "Sam", body: String? = "gm",
        pinnedBy: String? = nil
    ) throws -> ChatMessage {
        let base = ChatMessage(
            id: id, kind: .user,
            author: .init(id: author, handle: nil, displayName: name, photoUrl: nil),
            body: body, createdAt: Date(timeIntervalSince1970: 1_790_000_000),
            alsoInChannel: false, replyCount: 0, deleted: false, reactions: [], reactionCount: 0)
        guard let pinnedBy else { return base }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var object = try #require(JSONSerialization.jsonObject(with: encoder.encode(base)) as? [String: Any])
        object["pinned_at"] = "2026-11-04T12:00:00Z"
        object["pinned_by"] = ["id": "u-pinner", "handle": NSNull(), "display_name": pinnedBy, "photo_url": NSNull()]
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(ChatMessage.self, from: JSONSerialization.data(withJSONObject: object))
    }

    private func controls(role: String?) -> ChatPinControls {
        let transport = StubTransport(.json(.ok, "{}"))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return ChatPinControls(
            pins: ChatPinsModel(cabalID: "c1", viewerID: viewer, api: api), role: role, viewerID: viewer)
    }

    @Test func theCounterShowsOnlyWithTwoOrMorePins() {
        #expect(ChatPinCopy.counter(index: 0, count: 1) == nil)
        #expect(ChatPinCopy.counter(index: 0, count: 2) == "1 of 2")
        #expect(ChatPinCopy.counter(index: 2, count: 3) == "3 of 3")
    }

    @Test func thePinnerShowsByFirstName() throws {
        #expect(ChatPinCopy.pinner(of: try message(pinnedBy: "Christelle Dupont")) == "Christelle")
        #expect(ChatPinCopy.pinner(of: try message()) == "Someone")
        #expect(ChatPinCopy.firstName("  ") == "Someone")
    }

    @Test func theTextCollapsesWhitespace() throws {
        let text = ChatPinCopy.text(of: try message(body: "5 of 8\n to   pass"), cabalName: "AC", viewerID: viewer)
        #expect(text == "5 of 8 to pass")
    }

    @Test func voiceOverReadsThePinnerTheTextAndTheCounter() {
        #expect(
            ChatPinCopy.spokenLabel(pinner: "Christelle", text: "6 hour votes", counter: "1 of 2")
                == "Pinned by Christelle: 6 hour votes. 1 of 2.")
        #expect(
            ChatPinCopy.spokenLabel(pinner: "Christelle", text: "6 hour votes", counter: nil)
                == "Pinned by Christelle: 6 hour votes.")
    }

    @Test func adminsPinAnyMessageAndMembersOnlyTheirOwn() throws {
        let theirs = try message(author: "u-sam")
        let mine = try message(author: viewer, name: "Me")
        #expect(controls(role: "admin").canPin(theirs))
        #expect(controls(role: "member").canPin(mine))
        #expect(!controls(role: "member").canPin(theirs))
    }

    @Test func theCapsuleIsFortyPointsWithTargetsOfFortyFour() {
        #expect(ChatPinMetrics.height == 40)
        #expect(ChatPinMetrics.minTarget == 44)
        #expect(ChatPinMetrics.listRowLines == 3)
    }
}
