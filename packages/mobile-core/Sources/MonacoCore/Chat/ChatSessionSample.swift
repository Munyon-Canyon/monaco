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
    public var threaded = false
    public var neverPosts = false
    public var proposalCard = false
    public var moments = false
    public var poll = false
    public var pins = false

    public init(
        startEmpty: Bool = false,
        failSends: Bool = false,
        busy: Bool = false,
        closedFirstLoad: Bool = false,
        neverAnswers: Bool = false,
        failFirstSend: Bool = false,
        closedOnSend: Bool = false,
        threaded: Bool = false,
        neverPosts: Bool = false,
        proposalCard: Bool = false,
        moments: Bool = false,
        poll: Bool = false,
        pins: Bool = false
    ) {
        self.startEmpty = startEmpty
        self.failSends = failSends
        self.busy = busy
        self.closedFirstLoad = closedFirstLoad
        self.neverAnswers = neverAnswers
        self.failFirstSend = failFirstSend
        self.closedOnSend = closedOnSend
        self.threaded = threaded
        self.neverPosts = neverPosts
        self.proposalCard = proposalCard
        self.moments = moments
        self.poll = poll
        self.pins = pins
    }
}

extension ChatSession {
    public static let sampleCabalID = "00000000-0000-4000-8000-000000000166"
    public static let sampleViewerID = "u-me"
    public static let sampleProposalID = "00000000-0000-4000-8000-000000000501"

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

private struct SampleChatMissing: Error {}

private actor SampleChatStore {
    let scenario: ChatSampleScenario
    private var messages: [ChatMessage]
    private var listCalls = 0
    private var postCalls = 0
    private var arrivals = 0
    private var listeners: [UUID: AsyncStream<ChatRealtimeEvent>.Continuation] = [:]
    static let threadRootID = "root-1"

    init(scenario: ChatSampleScenario) {
        self.scenario = scenario
        if scenario.threaded {
            messages = [Self.message(Self.threadRootID, "u-ana", "Ana", "Apple reports Thursday.", minutesAgo: 10)]
        } else {
            messages =
                scenario.startEmpty
                ? []
                : Self.thread(
                    busy: scenario.busy, proposalCard: scenario.proposalCard, moments: scenario.moments,
                    poll: scenario.poll, pins: scenario.pins)
        }
    }

    func pinned() -> [ChatMessage] {
        guard scenario.pins else { return [] }
        return ["s0p", "s6"].compactMap { id in
            guard var message = messages.first(where: { $0.id == id }) else { return nil }
            message.pinnedBy = .init(id: "u-christelle", handle: nil, displayName: "Christelle", photoUrl: nil)
            return message
        }
    }

    func thread() -> (parent: ChatMessage, replies: [ChatMessage])? {
        guard let parent = messages.first(where: { $0.id == Self.threadRootID }) else { return nil }
        return (parent, messages.filter { $0.parentId == parent.id })
    }

    func page(before: String?, after: String?, limit: Int) async throws -> [ChatMessage] {
        if scenario.neverAnswers { try await Task.sleep(for: .seconds(3600)) }
        listCalls += 1
        if scenario.closedFirstLoad && listCalls == 1 { throw SampleChatRefusal() }
        let ordered =
            messages
            .filter { $0.parentId == nil || $0.alsoInChannel }
            .sorted { ($0.createdAt, $0.id) < ($1.createdAt, $1.id) }
        if let after, let index = ordered.firstIndex(where: { $0.id == after }) {
            return Array(ordered[(index + 1)...].prefix(limit))
        }
        let older = before.flatMap { id in ordered.firstIndex { $0.id == id } }.map { Array(ordered[..<$0]) } ?? ordered
        return Array(older.suffix(limit).reversed())
    }

    func post(body: String, parentID: String?, alsoInChannel: Bool) async throws -> ChatMessage {
        if scenario.neverPosts { try await Task.sleep(for: .seconds(3600)) }
        try await Task.sleep(for: .milliseconds(300))
        postCalls += 1
        if scenario.closedOnSend { throw SampleChatRefusal() }
        if scenario.failSends || (scenario.failFirstSend && postCalls == 1) {
            throw URLError(.notConnectedToInternet)
        }
        let id = "sent-\(messages.count + 1)"
        var message = Self.message(id, ChatSession.sampleViewerID, "You", body, minutesAgo: 0)
        message.parentId = parentID
        message.alsoInChannel = alsoInChannel
        messages.append(message)
        if let parentID, let index = messages.firstIndex(where: { $0.id == parentID }) {
            messages[index].replyCount += 1
            messages[index].lastReplyAt = message.createdAt
            let update = ChatRealtimeEvent.threadUpdated(
                id: parentID, replyCount: messages[index].replyCount, lastReplyAt: message.createdAt)
            for listener in listeners.values { listener.yield(update) }
        }
        return message
    }

    func post(poll input: Components.Schemas.PollAttachmentInput) async throws -> ChatMessage {
        try await Task.sleep(for: .milliseconds(300))
        let id = "sent-\(messages.count + 1)"
        var message = Self.message(id, ChatSession.sampleViewerID, "You", input.question, minutesAgo: 0)
        message.attachment = .poll(
            Components.Schemas.PollAttachment(
                kind: .poll, question: input.question,
                closesAt: Date(timeIntervalSinceNow: TimeInterval((input.closesInHours ?? 24) * 3_600)),
                closed: false,
                options: input.options.enumerated().map { .init(position: $0.offset, label: $0.element, count: 0) },
                total: 0))
        messages.append(message)
        return message
    }

    func vote(messageID: String, position: Int?) throws -> Components.Schemas.PollState {
        guard let index = messages.firstIndex(where: { $0.id == messageID }),
            case .poll(let poll) = messages[index].attachment
        else { throw SampleChatMissing() }
        let next = CommentPollModel.applying(position: position, to: poll)
        messages[index].attachment = .poll(next)
        let counts = next.options.map { ChatPollCount(position: $0.position, votes: $0.count) }
        let update = ChatRealtimeEvent.pollUpdated(messageId: messageID, options: counts, total: next.total)
        for listener in listeners.values { listener.yield(update) }
        return Components.Schemas.PollState(
            closesAt: next.closesAt, closed: next.closed, options: next.options, total: next.total,
            myPosition: next.myPosition)
    }

    func listen(_ continuation: AsyncStream<ChatRealtimeEvent>.Continuation) -> UUID {
        let id = UUID()
        listeners[id] = continuation
        return id
    }

    func stopListening(_ id: UUID) {
        listeners[id] = nil
    }

    func arrival() -> ChatMessage {
        arrivals += 1
        let body = "Still thinking about Thursday (\(arrivals))."
        let message = Self.message("incoming-\(arrivals)", "u-ana", "Ana", body, minutesAgo: 0)
        messages.append(message)
        return message
    }

    private static func thread(
        busy: Bool, proposalCard: Bool, moments: Bool, poll: Bool, pins: Bool
    ) -> [ChatMessage] {
        let me = ChatSession.sampleViewerID
        var thread = [
            message("s1", "u-ana", "Ana", "Apple reports Thursday. Anyone want in before?", minutesAgo: 1_210),
            message("s2", "u-ana", "Ana", "Thinking $50 from the pot.", minutesAgo: 1_209),
            message(
                "s3", "u-leo", "Leo", "Tesla instead? Or split it.", minutesAgo: 1_195,
                reactions: [.init(emoji: "😂", count: 3, mine: false), .init(emoji: "👍", count: 2, mine: false)]),
            message("s4", me, "You", "I'd rather do Apple first. Smaller swings for our first buy.", minutesAgo: 1_190),
            message("s5", "u-mia", "Mia", "Agree. Nvidia can be next.", minutesAgo: 95),
            message("s6", "u-ana", "Ana", "Proposing Apple now. $50.", minutesAgo: 12),
            message("s7", me, "You", "Voted yes.", minutesAgo: 9),
            message("s8", me, "You", "Leo, you're the last vote.", minutesAgo: 9),
        ]
        if proposalCard {
            var card = message("s5p", "u-mia", "Mia", "put it up 👇 small add before the print", minutesAgo: 80)
            card.proposalId = ChatSession.sampleProposalID
            card.replyCount = 6
            thread.insert(card, at: 5)
            var gif = message(
                "s5g", "u-sam", "Sam", GroupChatCopy.gifBody, minutesAgo: 70,
                reactions: [.init(emoji: "🔥", count: 2, mine: false)])
            let base = "https://\(SampleGifProvider.host)/sample-gif-4"
            gif.attachment = .gif(
                Components.Schemas.GifAttachment(
                    kind: .gif, id: "sample-gif-4", title: "Sample GIF 4",
                    still: .init(url: "\(base)/still.png", width: 200, height: 150),
                    preview: .init(url: "\(base)/preview.gif", width: 200, height: 150),
                    original: .init(url: "\(base)/original.gif", width: 200, height: 150)))
            thread.insert(gif, at: 6)
        }
        if pins {
            let rules = message(
                "s0p", "u-christelle", "Christelle", "5 of 8 to pass, 6 hour votes, max 2 open proposals at once",
                minutesAgo: 1_300)
            thread.insert(rules, at: 0)
        }
        if moments { thread = Self.moments() + thread }
        if poll {
            var question = message("s6q", "u-ana", "Ana", "Buy Apple before earnings?", minutesAgo: 11)
            question.attachment = .poll(
                Components.Schemas.PollAttachment(
                    kind: .poll, question: "Buy Apple before earnings?",
                    closesAt: Date(timeIntervalSinceNow: 86_400), closed: false,
                    options: [
                        .init(position: 0, label: "Yes", count: 3),
                        .init(position: 1, label: "Wait for earnings", count: 2),
                    ],
                    total: 5))
            thread.insert(question, at: 6)
        }
        if busy {
            thread += (1...60).map { index in
                let body = "Backlog line \(index) about the Apple buy."
                return message("b\(index)", "u-leo", "Leo", body, minutesAgo: 1_180 - index)
            }
        }
        return thread
    }

    private static func moments() -> [ChatMessage] {
        let created = system("mo1", .cabalCreated(.init(kind: .cabalCreated)), minutesAgo: 60 * 24 * 33 + 20)
        let joined = system(
            "mo2",
            .memberJoined(
                .init(
                    kind: .memberJoined,
                    member: .init(id: ChatSession.sampleViewerID, handle: nil, displayName: "You", photoUrl: nil))),
            minutesAgo: 60 * 24 * 33 + 10)
        var fill = system(
            "mo3",
            .tradeFilled(
                .init(
                    kind: .tradeFilled, side: .buy, symbol: "NVDAx", assetId: "sample-nvda", priceMicros: 196_750_000,
                    usdcMicros: 300_000_000, filledAt: Date(timeIntervalSinceNow: -33 * 86_400), swapId: "sample-swap",
                    proposalId: nil)),
            minutesAgo: 60 * 24 * 33)
        fill.reactions = [.init(emoji: "🔥", count: 2, mine: false)]
        fill.reactionCount = 2
        return [created, joined, fill]
    }

    private static func system(
        _ id: String, _ moment: Components.Schemas.ChatMoment, minutesAgo: Int
    ) -> ChatMessage {
        ChatMessage(
            id: id, kind: .system,
            author: .init(id: "00000000-0000-0000-0000-000000000000", handle: nil, displayName: "", photoUrl: nil),
            moment: moment, body: nil,
            createdAt: Date(timeIntervalSinceNow: -TimeInterval(minutesAgo * 60)),
            alsoInChannel: false, replyCount: 0, lastRepliers: [], deleted: false, reactions: [], reactionCount: 0)
    }

    private static func message(
        _ id: String, _ author: String, _ name: String, _ body: String, minutesAgo: Int,
        reactions: [Components.Schemas.Reaction] = []
    ) -> ChatMessage {
        ChatMessage(
            id: id, kind: .user,
            author: .init(id: author, handle: nil, displayName: name, photoUrl: nil),
            body: body,
            createdAt: Date(timeIntervalSinceNow: -TimeInterval(minutesAgo * 60)),
            alsoInChannel: false,
            replyCount: 0,
            lastRepliers: [], deleted: false,
            reactions: reactions,
            reactionCount: reactions.reduce(0) { $0 + $1.count }
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
            let listening = Task { await store.listen(continuation) }
            continuation.onTermination = { _ in
                arrivals.cancel()
                Task { await store.stopListening(await listening.value) }
            }
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
            switch operationID {
            case "postChatMessage":
                let data = try await Data(collecting: body ?? "", upTo: 8_192)
                let decoder = JSONDecoder()
                decoder.keyDecodingStrategy = .convertFromSnakeCase
                let draft = try decoder.decode(Draft.self, from: data)
                if let poll = draft.poll {
                    return try Self.reply(.created, try await store.post(poll: poll))
                }
                let stored = try await store.post(
                    body: draft.body ?? "", parentID: draft.parentId, alsoInChannel: draft.alsoInChannel ?? false)
                return try Self.reply(.created, stored)
            case "voteChatPoll":
                let data = try await Data(collecting: body ?? "", upTo: 1_024)
                let vote = try JSONDecoder().decode(Vote.self, from: data)
                return try Self.reply(
                    .ok, try await store.vote(messageID: Self.messageID(request), position: vote.position))
            case "unvoteChatPoll":
                return try Self.reply(.ok, try await store.vote(messageID: Self.messageID(request), position: nil))
            case "getChatThread":
                guard let thread = await store.thread() else { return Self.notFound() }
                return try Self.reply(.ok, Thread(parent: thread.parent, replies: thread.replies))
            case "getChatPins":
                return try Self.reply(.ok, Page(messages: await store.pinned()))
            case "getChatMessages":
                break
            default:
                return Self.notFound()
            }
            let query = URLComponents(string: request.path ?? "")?.queryItems ?? []
            func value(_ name: String) -> String? { query.first { $0.name == name }?.value }
            let page = try await store.page(
                before: value("before"), after: value("after"), limit: value("limit").flatMap(Int.init) ?? 50)
            return try Self.reply(.ok, Page(messages: page))
        } catch is SampleChatRefusal {
            return Self.refusal()
        } catch is SampleChatMissing {
            return Self.notFound()
        }
    }

    private static func messageID(_ request: HTTPRequest) -> String {
        let parts = (request.path ?? "").split(separator: "?").first.map { $0.split(separator: "/") } ?? []
        return parts.firstIndex(of: "messages").map { String(parts[$0 + 1]) } ?? ""
    }

    private struct Draft: Decodable {
        let body: String?
        let parentId: String?
        let alsoInChannel: Bool?
        let attachment: Attached?

        var poll: Components.Schemas.PollAttachmentInput? {
            guard let attachment, attachment.kind == "poll", let question = attachment.question,
                let options = attachment.options
            else { return nil }
            return .init(kind: .poll, question: question, options: options, closesInHours: attachment.closesInHours)
        }
    }

    private struct Attached: Decodable {
        let kind: String
        let question: String?
        let options: [String]?
        let closesInHours: Int?
    }

    private struct Vote: Decodable {
        let position: Int
    }

    private struct Thread: Encodable {
        let parent: ChatMessage
        let replies: [ChatMessage]
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

    private static func notFound() -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .notFound)
        response.headerFields[.contentType] = "application/problem+json"
        return (response, HTTPBody(#"{"type":"about:blank","title":"Not Found","status":404}"#))
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
