import Foundation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum PushRegistrationError: Error, Equatable, Sendable {
    case unknownEnvironment
}

public actor PushRegistrar {
    private struct Known: Equatable {
        let token: String
        var posted: Bool
    }

    private let service: any DeviceRegistering
    private let environment: PushEnvironment?
    private let sleep: @Sendable (Duration) async throws -> Void
    private var known: Known?

    public init(service: any DeviceRegistering, environment: PushEnvironment?, clock: some Clock<Duration>) {
        self.service = service
        self.environment = environment
        sleep = { try await clock.sleep(for: $0) }
    }

    public func register(token: String) async throws {
        guard let environment else { throw PushRegistrationError.unknownEnvironment }
        if known == Known(token: token, posted: true) { return }
        known = Known(token: token, posted: false)
        try await service.register(token: token, environment: environment)
        if known?.token == token { known?.posted = true }
    }

    public func unregister(within limit: Duration) async throws {
        guard let token = known?.token else { return }
        reset()
        let service = service
        let sleep = sleep
        try await withThrowingTaskGroup(of: Void.self) { group in
            group.addTask { try await service.unregister(token: token) }
            group.addTask {
                try await sleep(limit)
                throw URLError(.timedOut)
            }
            defer { group.cancelAll() }
            _ = try await group.next()
        }
    }

    public func reset() {
        known = nil
    }
}
