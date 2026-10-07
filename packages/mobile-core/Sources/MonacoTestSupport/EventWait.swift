import Foundation

public func eventArrives(within seconds: Double, _ event: @escaping @Sendable () async -> Void) async -> Bool {
    let once = OnceFlag()
    return await withCheckedContinuation { continuation in
        Task {
            await event()
            if once.claim() { continuation.resume(returning: true) }
        }
        Task {
            try? await Task.sleep(nanoseconds: UInt64(seconds * 1_000_000_000))
            if once.claim() { continuation.resume(returning: false) }
        }
    }
}

private final class OnceFlag: @unchecked Sendable {
    private let lock = NSLock()
    private var claimed = false

    func claim() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if claimed { return false }
        claimed = true
        return true
    }
}
