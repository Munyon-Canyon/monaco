import Ably
import Foundation
import MonacoCore
import Testing

@testable import Monaco

struct AblyChatRealtimeTests {
    @Test func aCabalJoinedAfterTheTokenWasIssuedReauthorizesOnceAndGetsLiveMessages() async {
        let ably = FakeAbly(memberOf: ["a"])
        let link = AblyChatRealtime { ably }
        let first = Recorder(link.events(cabalId: "a"))
        #expect(await eventually { first.events == [.attached(resumed: false)] })

        ably.join("b")
        let joined = Recorder(link.events(cabalId: "b"))
        #expect(await eventually { joined.events == [.detached, .attached(resumed: false)] })
        ably.publish(deletedMessage: "m1", in: "b")

        #expect(await eventually { joined.events.last == .messageDeleted(id: "m1") })
        await settle()
        #expect(joined.events == [.detached, .attached(resumed: false), .messageDeleted(id: "m1")])
        #expect(ably.tokens == 2)
        #expect(ably.channel("b").attaches == 2)
    }

    @Test func aRefusedCabalReauthorizesOncePerOpenAndAttachesOnceJoined() async {
        let ably = FakeAbly(memberOf: [])
        let link = AblyChatRealtime { ably }
        let refused = Recorder(link.events(cabalId: "c"))
        #expect(await eventually { refused.events == [.detached, .detached] })
        await settle()
        #expect(ably.tokens == 2)
        #expect(ably.channel("c").attaches == 2)

        refused.stop()
        #expect(await eventually { !ably.channel("c").hasListeners })
        ably.join("c")
        let reopened = Recorder(link.events(cabalId: "c"))
        #expect(await eventually { reopened.events == [.attached(resumed: false)] })
        ably.publish(deletedMessage: "m1", in: "c")

        #expect(await eventually { reopened.events.last == .messageDeleted(id: "m1") })
        await settle()
        #expect(reopened.events == [.attached(resumed: false), .messageDeleted(id: "m1")])
        #expect(ably.tokens == 3)
        #expect(ably.channel("c").attaches == 3)
    }

    private func eventually(_ condition: () -> Bool) async -> Bool {
        for _ in 0..<1000 {
            if condition() { return true }
            await Task.yield()
        }
        return condition()
    }

    private func settle() async {
        for _ in 0..<200 { await Task.yield() }
    }
}

private final class Recorder {
    private(set) var events: [ChatRealtimeEvent] = []
    private var task: Task<Void, Never>?

    init(_ stream: AsyncStream<ChatRealtimeEvent>) {
        task = Task { [weak self] in
            for await event in stream { self?.events.append(event) }
        }
    }

    func stop() { task?.cancel() }
}

private let capabilityRefused = Int(ARTErrorCode.operationNotPermittedWithProvidedCapability.rawValue)

private final class FakeAbly: AblyClient {
    private var members: Set<String>
    private var granted: Set<String>
    private var channels: [String: FakeChannel] = [:]
    private(set) var tokens = 1

    init(memberOf cabals: Set<String>) {
        members = Set(cabals.map { "cabal:\($0)" })
        granted = members
    }

    func join(_ cabal: String) { members.insert("cabal:\(cabal)") }

    func channel(_ cabal: String) -> FakeChannel { fakeChannel(named: "cabal:\(cabal)") }

    func publish(deletedMessage id: String, in cabal: String) {
        channel(cabal).deliver(ARTMessage(name: ChatEventDecoder.messageDeleted, data: #"{"id":"\#(id)"}"#))
    }

    func allows(_ name: String) -> Bool { granted.contains(name) }

    func channel(named name: String) -> any AblyChannel { fakeChannel(named: name) }

    func reauthorize() async {
        tokens += 1
        granted = members
    }

    func close() {}

    private func fakeChannel(named name: String) -> FakeChannel {
        if let existing = channels[name] { return existing }
        let created = FakeChannel(name: name, ably: self)
        channels[name] = created
        return created
    }
}

private final class FakeChannel: AblyChannel {
    private let name: String
    private unowned let ably: FakeAbly
    private var stateListeners: [(ARTEventListener, (ARTChannelStateChange) -> Void)] = []
    private var messageListeners: [(ARTEventListener, ARTMessageCallback)] = []
    private(set) var state: ARTRealtimeChannelState = .initialized
    private(set) var attaches = 0

    init(name: String, ably: FakeAbly) {
        self.name = name
        self.ably = ably
    }

    var hasListeners: Bool { !stateListeners.isEmpty || !messageListeners.isEmpty }

    func on(_ callback: @escaping (ARTChannelStateChange) -> Void) -> ARTEventListener {
        let listener = ARTEventListener()
        stateListeners.append((listener, callback))
        return listener
    }

    func subscribe(_ callback: @escaping ARTMessageCallback) -> ARTEventListener? {
        guard state != .failed else { return nil }
        let listener = ARTEventListener()
        messageListeners.append((listener, callback))
        if state == .initialized || state == .detached { attach() }
        return listener
    }

    func unsubscribe(_ listener: ARTEventListener?) { messageListeners.removeAll { $0.0 === listener } }

    func off(_ listener: ARTEventListener) { stateListeners.removeAll { $0.0 === listener } }

    func attach() {
        attaches += 1
        if ably.allows(name) {
            move(to: .attached, reason: nil)
        } else {
            move(to: .failed, reason: ARTErrorInfo.create(withCode: capabilityRefused, message: "not permitted"))
        }
    }

    func detach() {
        guard state != .failed else { return }
        move(to: .detached, reason: nil)
    }

    func deliver(_ message: ARTMessage) {
        guard state == .attached else { return }
        for (_, callback) in messageListeners { callback(message) }
    }

    private func move(to next: ARTRealtimeChannelState, reason: ARTErrorInfo?) {
        let change = ARTChannelStateChange(
            current: next, previous: state, event: ARTChannelEvent(rawValue: next.rawValue) ?? .update, reason: reason)
        state = next
        let listeners = stateListeners.map(\.1)
        Task { for listener in listeners { listener(change) } }
    }
}
