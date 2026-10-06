#if DEBUG
import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct ChatSampleScenario: Sendable, Equatable {
    public var startEmpty = false
    public var failSends = false
    public var busy = false
    public var closedFirstLoad = false
    public var neverAnswers = false
    public var failFirstSend = false
    public var closedOnSend = false

    public init(
        startEmpty: Bool = false,
        failSends: Bool = false,
        busy: Bool = false,
        closedFirstLoad: Bool = false,
        neverAnswers: Bool = false,
        failFirstSend: Bool = false,
        closedOnSend: Bool = false
    ) {
        self.startEmpty = startEmpty
        self.failSends = failSends
        self.busy = busy
        self.closedFirstLoad = closedFirstLoad
        self.neverAnswers = neverAnswers
        self.failFirstSend = failFirstSend
        self.closedOnSend = closedOnSend
    }
}

extension ChatSession {
    public static let sampleCabalID = "00000000-0000-4000-8000-000000000166"
    public static let sampleViewerID = "u-me"

    public static func sample(_ scenario: ChatSampleScenario, now: @escaping @Sendable () -> Date) -> ChatSession {
        let store = SampleChatStore(scenario: scenario)
        return ChatSession(
            cabalID: sampleCabalID,
            viewerID: sampleViewerID,
            api: APIClient(
                serverURL: URL(fileURLWithPath: "/"),
                tokens: SampleChatTokens(),
                transport: SampleChatTransport(store: store)
            ),
            realtime: SampleChatRealtime(store: store),
            now: now
        )
    }
}

private struct SampleChatTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "sample-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct SampleChatRefusal: Error {}

private actor SampleChatStore {
    let scenario: ChatSampleScenario
    private var messages: [ChatMessage]
    private var listCalls = 0
    private var postCalls = 0
    private var arrivals = 0

    init(scenario: ChatSampleScenario) {
        self.scenario = scenario
        messages = scenario.startEmpty ? [] : Self.thread(busy: scenario.busy)
    }

    func page(before: String?, after: String?, limit: Int) async throws -> [ChatMessage] {
        if scenario.neverAnswers { try await Task.sleep(for: .seconds(3600)) }
        listCalls += 1
        if scenario.closedFirstLoad && listCalls == 1 { throw SampleChatRefusal() }
        let ordered = messages.sorted { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }
        if let after, let index = ordered.firstIndex(where: { $0.id == after }) {
            return Array(ordered[(index + 1)...].prefix(limit))
        }
        let older = before.flatMap { id in ordered.firstIndex { $0.id == id } }.map { Array(ordered[..<$0]) } ?? ordered
        return Array(older.suffix(limit).reversed())
    }

    func post(body: String) async throws -> ChatMessage {
        try await Task.sleep(for: .milliseconds(300))
        postCalls += 1
        if scenario.closedOnSend { throw SampleChatRefusal() }
        if scenario.failSends || (scenario.failFirstSend && postCalls == 1) {
            throw URLError(.notConnectedToInternet)
        }
        let message = Self.message("sent-\(messages.count + 1)", ChatSession.sampleViewerID, "You", body, minutesAgo: 0)
        messages.append(message)
        return message
    }

    func arrival() -> ChatMessage {
        arrivals += 1
        let body = "Still thinking about Thursday (\(arrivals))."
        let message = Self.message("incoming-\(arrivals)", "u-ana", "Ana", body, minutesAgo: 0)
        messages.append(message)
        return message
    }

    private static func thread(busy: Bool) -> [ChatMessage] {
        let me = ChatSession.sampleViewerID
        var thread = [
            message("s1", "u-ana", "Ana", "Apple reports Thursday. Anyone want in before?", minutesAgo: 1_210),
            message("s2", "u-ana", "Ana", "Thinking $50 from the pot.", minutesAgo: 1_209),
            message("s3", "u-leo", "Leo", "Tesla instead? Or split it.", minutesAgo: 1_195),
            message("s4", me, "You", "I'd rather do Apple first. Smaller swings for our first buy.", minutesAgo: 1_190),
            message("s5", "u-mia", "Mia", "Agree. Nvidia can be next.", minutesAgo: 95),
            message("s6", "u-ana", "Ana", "Proposing Apple now. $50.", minutesAgo: 12),
            message("s7", me, "You", "Voted yes.", minutesAgo: 9),
            message("s8", me, "You", "Leo, you're the last vote.", minutesAgo: 9),
        ]
        if busy {
            thread += (1...60).map { index in
                let body = "Backlog line \(index) about the Apple buy."
                return message("b\(index)", "u-leo", "Leo", body, minutesAgo: 1_180 - index)
            }
        }
        return thread
    }

    private static func message(_ id: String, _ author: String, _ name: String, _ body: String, minutesAgo: Int)
        -> ChatMessage
    {
        ChatMessage(
            id: id,
            author: .init(id: author, handle: nil, displayName: name, photoUrl: nil),
            body: body,
            createdAt: Date(timeIntervalSinceNow: -TimeInterval(minutesAgo * 60)),
            alsoInChannel: false,
            replyCount: 0,
            deleted: false
        )
    }
}

private struct SampleChatRealtime: ChatRealtime {
    let store: SampleChatStore

    func events(cabalId _: String) -> AsyncStream<ChatRealtimeEvent> {
        let store = store
        return AsyncStream { continuation in
            continuation.yield(.attached(resumed: true))
            let arrivals = Task {
                guard store.scenario.busy else { return }
                while !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(4))
                    continuation.yield(.messageCreated(await store.arrival()))
                }
            }
            continuation.onTermination = { _ in arrivals.cancel() }
        }
    }

    func detach(cabalId _: String) {}
}

private struct SampleChatTransport: ClientTransport {
    let store: SampleChatStore

    func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        do {
            if operationID == "postChatMessage" {
                let data = try await Data(collecting: body ?? "", upTo: 8_192)
                let draft = try JSONDecoder().decode(Draft.self, from: data)
                return try Self.reply(.created, try await store.post(body: draft.body))
            }
            let query = URLComponents(string: request.path ?? "")?.queryItems ?? []
            func value(_ name: String) -> String? { query.first { $0.name == name }?.value }
            let page = try await store.page(
                before: value("before"), after: value("after"), limit: value("limit").flatMap(Int.init) ?? 50)
            return try Self.reply(.ok, Page(messages: page))
        } catch is SampleChatRefusal {
            return Self.refusal()
        }
    }

    private struct Draft: Decodable {
        let body: String
    }

    private struct Page: Encodable {
        let messages: [ChatMessage]
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

    private static func refusal() -> (HTTPResponse, HTTPBody?) {
        let problem =
            #"{"type":"about:blank","title":"Forbidden","status":403,"code":"not_cabal_member","#
            + #""message":"You are not in this cabal.","trace_id":"00000000000000000000000000000000","retryable":false}"#
        var response = HTTPResponse(status: .forbidden)
        response.headerFields[.contentType] = "application/problem+json"
        return (response, HTTPBody(problem))
    }
}
#endif
