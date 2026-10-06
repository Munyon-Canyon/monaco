import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import Synchronization

final class FakeChatRealtime: ChatRealtime {
    private struct Store {
        var subscribed: [String] = []
        var detached: [String] = []
        var continuation: AsyncStream<ChatRealtimeEvent>.Continuation?
    }

    private let store = Mutex(Store())

    var subscribed: [String] { store.withLock { $0.subscribed } }
    var detached: [String] { store.withLock { $0.detached } }

    func events(cabalId: String) -> AsyncStream<ChatRealtimeEvent> {
        let (stream, continuation) = AsyncStream.makeStream(of: ChatRealtimeEvent.self)
        store.withLock {
            $0.subscribed.append(cabalId)
            $0.continuation = continuation
        }
        return stream
    }

    func detach(cabalId: String) {
        store.withLock {
            $0.detached.append(cabalId)
            $0.continuation?.finish()
            $0.continuation = nil
        }
    }

    func emit(_ event: ChatRealtimeEvent) {
        store.withLock { _ = $0.continuation?.yield(event) }
    }
}

extension ChatFixtures {
    static func page(_ messages: [ChatMessage]) throws -> StubTransport.Reply {
        .json(.ok, #"{"messages":[\#(try messages.map(json).joined(separator: ","))]}"#)
    }

    static func created(_ message: ChatMessage) throws -> StubTransport.Reply {
        .json(.created, try json(message))
    }

    static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }

    static func session(
        _ transport: StubTransport,
        realtime: FakeChatRealtime = FakeChatRealtime(),
        keys: [String] = ["key-1", "key-2", "key-3"]
    ) -> ChatSession {
        let remaining = Mutex(keys)
        return ChatSession(
            cabalID: cabalID,
            viewerID: viewerID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            realtime: realtime,
            now: { epoch.addingTimeInterval(3_600) },
            makeKey: { remaining.withLock { $0.isEmpty ? "key-extra" : $0.removeFirst() } }
        )
    }

    static func state(_ session: ChatSession) async -> ChatSession.State {
        for await state in await session.states() { return state }
        preconditionFailure("states ended")
    }

    static func ids(_ state: ChatSession.State) -> [String] {
        state.timeline.rows.map(\.id)
    }

    static func idempotencyKeys(_ transport: StubTransport) async -> [String] {
        await transport.sent.compactMap { request in
            request.headerFields.first { $0.name.canonicalName == IdempotentSubmission.keyHeader.lowercased() }?.value
        }
    }
}
