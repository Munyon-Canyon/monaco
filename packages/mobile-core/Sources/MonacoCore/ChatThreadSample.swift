#if DEBUG
import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

extension ChatSession {
    public static func threadSample(now: @escaping @Sendable () -> Date) -> ChatSession {
        ChatSession(
            cabalID: sampleCabalID,
            viewerID: sampleViewerID,
            api: APIClient(
                serverURL: URL(fileURLWithPath: "/"),
                tokens: ThreadSampleTokens(),
                transport: ThreadSampleTransport(store: ThreadSampleStore(now: now))
            ),
            realtime: ThreadSampleRealtime(),
            now: now
        )
    }
}

private struct ThreadSampleTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "sample-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct ThreadSampleRealtime: ChatRealtime {
    func events(cabalId _: String) -> AsyncStream<ChatRealtimeEvent> {
        AsyncStream { continuation in continuation.yield(.attached(resumed: true)) }
    }

    func detach(cabalId _: String) {}
}

private actor ThreadSampleStore {
    private let now: @Sendable () -> Date
    static let rootID = "root-1"
    private var messages: [ChatMessage]

    init(now: @escaping @Sendable () -> Date) {
        self.now = now
        messages = [
            ChatMessage(
                id: Self.rootID,
                author: .init(id: "u-ana", handle: nil, displayName: "Ana", photoUrl: nil),
                body: "Apple reports Thursday.",
                createdAt: now().addingTimeInterval(-600),
                alsoInChannel: false,
                replyCount: 0,
                deleted: false)
        ]
    }

    func channel() -> [ChatMessage] {
        messages.filter { $0.parentId == nil || $0.alsoInChannel }.reversed()
    }

    func thread() -> (parent: ChatMessage, replies: [ChatMessage]) {
        (messages[0], messages.filter { $0.parentId == Self.rootID })
    }

    func post(body: String, parentID: String?, alsoInChannel: Bool) -> ChatMessage {
        let message = ChatMessage(
            id: "sent-\(messages.count + 1)",
            author: .init(id: "u-me", handle: nil, displayName: "You", photoUrl: nil),
            body: body,
            createdAt: now(),
            parentId: parentID,
            alsoInChannel: alsoInChannel,
            replyCount: 0,
            deleted: false)
        messages.append(message)
        if parentID != nil {
            messages[0].replyCount += 1
            messages[0].lastReplyAt = message.createdAt
        }
        return message
    }
}

private struct ThreadSampleTransport: ClientTransport {
    let store: ThreadSampleStore

    private struct Draft: Decodable {
        let body: String
        let parentId: String?
        let alsoInChannel: Bool?
    }

    private struct Page: Encodable {
        let messages: [ChatMessage]
    }

    private struct Thread: Encodable {
        let parent: ChatMessage
        let replies: [ChatMessage]
    }

    func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch operationID {
        case "postChatMessage":
            let data = try await Data(collecting: body ?? "", upTo: 8_192)
            let decoder = JSONDecoder()
            decoder.keyDecodingStrategy = .convertFromSnakeCase
            let draft = try decoder.decode(Draft.self, from: data)
            let stored = await store.post(
                body: draft.body, parentID: draft.parentId, alsoInChannel: draft.alsoInChannel ?? false)
            return try Self.reply(.created, stored)
        case "getChatThread":
            let thread = await store.thread()
            return try Self.reply(.ok, Thread(parent: thread.parent, replies: thread.replies))
        default:
            let query = URLComponents(string: request.path ?? "")?.queryItems ?? []
            let isNewer = query.contains { $0.name == "after" }
            return try Self.reply(.ok, Page(messages: isNewer ? [] : await store.channel()))
        }
    }

    private static func reply(_ status: HTTPResponse.Status, _ value: some Encodable) throws -> (
        HTTPResponse, HTTPBody?
    ) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(value)))
    }
}
#endif
