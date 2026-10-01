import Foundation

/// Collapses concurrent calls into one piece of work. Several requests usually fail
/// with 401 at the same moment when a token expires; they should share one refresh.
public actor SingleFlight<Value: Sendable> {
    private var inFlight: Task<Value, Error>?
    private var waiting = 0

    public init() {}

    /// Callers blocked on the in-flight run. A test waits for this instead of sleeping.
    public var waiters: Int { waiting }

    public func run(_ work: @escaping @Sendable () async throws -> Value) async throws -> Value {
        if let inFlight {
            waiting += 1
            defer { waiting -= 1 }
            return try await inFlight.value
        }
        let task = Task { try await work() }
        inFlight = task
        defer { inFlight = nil }
        return try await task.value
    }
}
