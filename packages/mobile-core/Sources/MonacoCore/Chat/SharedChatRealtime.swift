import Foundation
import Synchronization

public protocol ChatRealtimeLink: ChatRealtime {
    func close()
}

public final class SharedChatRealtime: ChatRealtime {
    private let connect: @Sendable () -> any ChatRealtimeLink
    private let link = Mutex<(any ChatRealtimeLink)?>(nil)

    public init(connect: @escaping @Sendable () -> any ChatRealtimeLink) {
        self.connect = connect
    }

    public func events(cabalId: String) -> AsyncStream<ChatRealtimeEvent> {
        current().events(cabalId: cabalId)
    }

    public func detach(cabalId: String) {
        current().detach(cabalId: cabalId)
    }

    public func close() {
        let closing = link.withLock { held -> (any ChatRealtimeLink)? in
            defer { held = nil }
            return held
        }
        closing?.close()
    }

    private func current() -> any ChatRealtimeLink {
        link.withLock { held in
            if let held { return held }
            let created = connect()
            held = created
            return created
        }
    }
}
