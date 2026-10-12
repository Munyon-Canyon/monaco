import Foundation
import MonacoAPI
import Observation

@Observable @MainActor
public final class AssetBestChainStore {
    public static let refreshAfter: TimeInterval = 60

    private struct Entry {
        let bestChain: AssetBestChain
        let receivedAt: Date
    }

    private var entries: [String: Entry] = [:]
    @ObservationIgnored private var inFlight: [String: Task<Void, Never>] = [:]
    @ObservationIgnored private let fetch: @Sendable (String) async throws -> AssetBestChain
    @ObservationIgnored private let now: @Sendable () -> Date

    public init(
        now: @escaping @Sendable () -> Date,
        fetch: @escaping @Sendable (_ symbol: String) async throws -> AssetBestChain
    ) {
        self.now = now
        self.fetch = fetch
    }

    public convenience init(api: APIClient, now: @escaping @Sendable () -> Date) {
        self.init(
            now: now,
            fetch: { symbol in
                let wire = try await api.read {
                    try await $0.getAssetBestChain(path: .init(symbol: symbol)).ok.body.json
                }
                guard let bestChain = AssetBestChain(wire) else { throw APIError.decoding("AssetBestChain") }
                return bestChain
            })
    }

    public func bestChain(for symbol: String) -> AssetBestChain? {
        guard let entry = entries[Self.key(symbol)], isFresh(entry) else { return nil }
        return entry.bestChain
    }

    public func refreshIfNeeded(symbol: String) async {
        let key = Self.key(symbol)
        if let entry = entries[key], isFresh(entry) { return }
        if let running = inFlight[key] {
            await running.value
            return
        }
        let task = Task { [self] in
            defer { inFlight[key] = nil }
            do {
                let bestChain = try await fetch(symbol)
                entries[key] = Entry(bestChain: bestChain, receivedAt: now())
            } catch {
                if let entry = entries[key], !isFresh(entry) { entries[key] = nil }
            }
        }
        inFlight[key] = task
        await task.value
    }

    public func reset() {
        for task in inFlight.values { task.cancel() }
        inFlight = [:]
        entries = [:]
    }

    private func isFresh(_ entry: Entry) -> Bool {
        let current = now()
        let age = max(current.timeIntervalSince(entry.receivedAt), current.timeIntervalSince(entry.bestChain.quotedAt))
        return age < Self.refreshAfter
    }

    private static func key(_ symbol: String) -> String { symbol.uppercased() }
}
