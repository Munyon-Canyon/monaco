import Ably
import Foundation
import MonacoAPI
import MonacoCore

final class AblyChatRealtime: ChatRealtime {
    private let connection: AblyChatConnection

    init(api: APIClient) {
        connection = AblyChatConnection(api: api)
    }

    nonisolated func events(cabalId: String) -> AsyncStream<ChatRealtimeEvent> {
        let (stream, continuation) = AsyncStream.makeStream(of: ChatRealtimeEvent.self)
        let subscription = UUID()
        let connection = connection
        continuation.onTermination = { _ in
            Task { @MainActor in connection.unsubscribe(cabalId, only: subscription) }
        }
        Task { @MainActor in connection.subscribe(cabalId, id: subscription, into: continuation) }
        return stream
    }

    nonisolated func detach(cabalId: String) {
        let connection = connection
        Task { @MainActor in connection.unsubscribe(cabalId, only: nil) }
    }
}

private final class AblyChatConnection {
    private struct Subscription {
        let id: UUID
        let channel: ARTRealtimeChannel
        let messages: ARTEventListener?
        let states: ARTEventListener
        let continuation: AsyncStream<ChatRealtimeEvent>.Continuation
    }

    private let api: APIClient
    private var realtime: ARTRealtime?
    private var subscriptions: [String: Subscription] = [:]

    nonisolated init(api: APIClient) {
        self.api = api
    }

    func subscribe(_ cabalId: String, id: UUID, into continuation: AsyncStream<ChatRealtimeEvent>.Continuation) {
        unsubscribe(cabalId, only: nil)
        let realtime = connectedRealtime()
        let channel = realtime.channels.get("cabal:\(cabalId)")
        let states = channel.on(Self.stateHandler(continuation))
        let messages = channel.subscribe(Self.messageHandler(continuation))
        subscriptions[cabalId] = Subscription(
            id: id, channel: channel, messages: messages, states: states, continuation: continuation)
    }

    func unsubscribe(_ cabalId: String, only id: UUID?) {
        guard let subscription = subscriptions[cabalId], id == nil || subscription.id == id else { return }
        subscriptions[cabalId] = nil
        if let messages = subscription.messages { subscription.channel.unsubscribe(messages) }
        subscription.channel.off(subscription.states)
        subscription.channel.detach()
        subscription.continuation.finish()
        guard subscriptions.isEmpty else { return }
        realtime?.close()
        realtime = nil
    }

    private func connectedRealtime() -> ARTRealtime {
        if let realtime { return realtime }
        let options = ARTClientOptions()
        options.authCallback = Self.authCallback(api: api)
        let created = ARTRealtime(options: options)
        realtime = created
        return created
    }

    private nonisolated static func authCallback(api: APIClient) -> ARTAuthCallback {
        { _, callback in
            Task {
                do {
                    let request = try await api.submit(
                        IdempotentSubmission(), payload: "realtime-token", operation: "createRealtimeToken"
                    ) { client, key in
                        try await client.createRealtimeToken(headers: .init(idempotencyKey: key)).ok.body.json
                    }
                    let json = String(decoding: try JSONEncoder().encode(request), as: UTF8.self)
                    callback(try ARTTokenRequest.fromJson(json as NSString), nil)
                } catch {
                    callback(nil, error as NSError)
                }
            }
        }
    }

    private nonisolated static func messageHandler(
        _ continuation: AsyncStream<ChatRealtimeEvent>.Continuation
    ) -> ARTMessageCallback {
        { message in
            guard let name = message.name, let data = payload(message.data),
                let event = ChatEventDecoder.decode(name: name, data: data)
            else { return }
            continuation.yield(event)
        }
    }

    private nonisolated static func stateHandler(
        _ continuation: AsyncStream<ChatRealtimeEvent>.Continuation
    ) -> (ARTChannelStateChange) -> Void {
        { change in
            switch change.current {
            case .attached:
                continuation.yield(.attached(resumed: change.resumed))
            case .detached, .suspended, .failed:
                continuation.yield(.detached)
            default:
                return
            }
        }
    }

    private nonisolated static func payload(_ data: Any?) -> Data? {
        switch data {
        case let text as String:
            return Data(text.utf8)
        case let raw as Data:
            return raw
        case let object? where JSONSerialization.isValidJSONObject(object):
            return try? JSONSerialization.data(withJSONObject: object)
        default:
            return nil
        }
    }
}
