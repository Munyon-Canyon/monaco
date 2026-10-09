import Ably
import Foundation
import MonacoAPI
import MonacoCore

protocol AblyClient: AnyObject {
    func channel(named name: String) -> any AblyChannel
    func reauthorize() async
    func close()
}

protocol AblyChannel: AnyObject {
    var state: ARTRealtimeChannelState { get }
    func on(_ callback: @escaping (ARTChannelStateChange) -> Void) -> ARTEventListener
    func subscribe(_ callback: @escaping ARTMessageCallback) -> ARTEventListener?
    func unsubscribe(_ listener: ARTEventListener?)
    func off(_ listener: ARTEventListener)
    func attach()
    func detach()
}

extension ARTRealtimeChannel: AblyChannel {}

extension ARTRealtime: AblyClient {
    func channel(named name: String) -> any AblyChannel {
        channels.get(name)
    }

    func reauthorize() async {
        await withCheckedContinuation { continuation in
            auth.authorize { _, _ in continuation.resume() }
        }
    }
}

final class AblyChatRealtime: ChatRealtimeLink {
    private let connection: AblyChatConnection

    convenience init(api: APIClient) {
        self.init {
            let options = ARTClientOptions()
            options.authCallback = AblyChatRealtime.authCallback(api: api)
            return ARTRealtime(options: options)
        }
    }

    init(connect: @escaping @MainActor () -> any AblyClient) {
        connection = AblyChatConnection(connect: connect)
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

    nonisolated func close() {
        let connection = connection
        Task { @MainActor in connection.closeAll() }
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
}

private final class AblyChatConnection {
    private final class CabalChannel {
        let channel: any AblyChannel
        private let client: any AblyClient
        var messages: ARTEventListener?
        var states: ARTEventListener?
        private var reauthorized = false
        var subscribers: [UUID: AsyncStream<ChatRealtimeEvent>.Continuation] = [:]

        init(_ cabalId: String, on client: any AblyClient) {
            self.client = client
            channel = client.channel(named: "cabal:\(cabalId)")
        }

        func yield(_ event: ChatRealtimeEvent) {
            for continuation in subscribers.values { continuation.yield(event) }
        }

        func attachAndSubscribe() {
            if channel.state != .failed {
                subscribeOnce()
            } else if !reauthorized {
                reauthorized = true
                Task { [weak self, client] in
                    await client.reauthorize()
                    guard let self else { return }
                    channel.attach()
                    subscribeOnce()
                }
            }
        }

        private func subscribeOnce() {
            guard messages == nil else { return }
            messages = channel.subscribe(AblyChatConnection.messageHandler { [weak self] event in self?.yield(event) })
        }
    }

    private let connect: @MainActor () -> any AblyClient
    private var client: (any AblyClient)?
    private var channels: [String: CabalChannel] = [:]

    nonisolated init(connect: @escaping @MainActor () -> any AblyClient) {
        self.connect = connect
    }

    func subscribe(_ cabalId: String, id: UUID, into continuation: AsyncStream<ChatRealtimeEvent>.Continuation) {
        if let existing = channels[cabalId] {
            existing.subscribers[id] = continuation
            if existing.channel.state == .attached {
                continuation.yield(.attached(resumed: false))
            } else {
                existing.attachAndSubscribe()
            }
            return
        }
        let cabal = CabalChannel(cabalId, on: connectedClient())
        cabal.subscribers[id] = continuation
        cabal.states = cabal.channel.on(
            Self.stateHandler(
                emit: { [weak cabal] event in cabal?.yield(event) },
                refused: { [weak cabal] in cabal?.attachAndSubscribe() }
            ))
        channels[cabalId] = cabal
        cabal.attachAndSubscribe()
    }

    func unsubscribe(_ cabalId: String, only id: UUID?) {
        guard let cabal = channels[cabalId] else { return }
        var finished: [AsyncStream<ChatRealtimeEvent>.Continuation] = []
        if let id {
            if let continuation = cabal.subscribers.removeValue(forKey: id) { finished = [continuation] }
        } else {
            finished = Array(cabal.subscribers.values)
            cabal.subscribers = [:]
        }
        if cabal.subscribers.isEmpty {
            channels[cabalId] = nil
            if let messages = cabal.messages { cabal.channel.unsubscribe(messages) }
            if let states = cabal.states { cabal.channel.off(states) }
            cabal.channel.detach()
        }
        for continuation in finished { continuation.finish() }
    }

    func closeAll() {
        for cabalId in Array(channels.keys) { unsubscribe(cabalId, only: nil) }
        client?.close()
        client = nil
    }

    private func connectedClient() -> any AblyClient {
        if let client { return client }
        let created = connect()
        client = created
        return created
    }

    private nonisolated static func messageHandler(
        _ emit: @escaping (ChatRealtimeEvent) -> Void
    ) -> ARTMessageCallback {
        { message in
            guard let name = message.name, let data = payload(message.data),
                let event = ChatEventDecoder.decode(name: name, data: data)
            else { return }
            emit(event)
        }
    }

    private nonisolated static let capabilityRefused = Int(
        ARTErrorCode.operationNotPermittedWithProvidedCapability.rawValue)

    private nonisolated static func stateHandler(
        emit: @escaping (ChatRealtimeEvent) -> Void,
        refused: @escaping () -> Void
    ) -> (ARTChannelStateChange) -> Void {
        { change in
            switch change.current {
            case .attached:
                emit(.attached(resumed: change.resumed))
            case .failed:
                emit(.detached)
                if change.reason?.code == capabilityRefused { refused() }
            case .detached, .suspended:
                emit(.detached)
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
