import Foundation
import MonacoAPI

public actor AssetLogoStore {
    static let maxInFlight = 4

    private let api: APIClient
    private var resolved: [String: URL?] = [:]
    private var lookups: [String: Task<URL?, Never>] = [:]
    private var running = 0
    private var waiters: [CheckedContinuation<Void, Never>] = []

    public init(api: APIClient) {
        self.api = api
    }

    public func logos(for symbols: [String]) async -> [String: URL] {
        await withTaskGroup(of: (String, URL?).self) { group in
            for symbol in Set(symbols) {
                group.addTask { (symbol, await self.logo(for: symbol)) }
            }
            var found: [String: URL] = [:]
            for await (symbol, url) in group {
                if let url { found[symbol] = url }
            }
            return found
        }
    }

    private func logo(for symbol: String) async -> URL? {
        if let known = resolved[symbol] { return known }
        if let pending = lookups[symbol] { return await pending.value }
        let task = Task { await self.lookUp(symbol) }
        lookups[symbol] = task
        return await task.value
    }

    private func lookUp(_ symbol: String) async -> URL? {
        await acquire()
        let url = await fetch(symbol)
        release()
        resolved[symbol] = .some(url)
        lookups[symbol] = nil
        return url
    }

    private func fetch(_ symbol: String) async -> URL? {
        let asset = try? await api.read { client in
            try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
        }
        guard let raw = asset?.logoUrl, let url = URL(string: raw), url.scheme != nil else { return nil }
        return url
    }

    private func acquire() async {
        if running < Self.maxInFlight {
            running += 1
            return
        }
        await withCheckedContinuation { waiters.append($0) }
    }

    private func release() {
        if waiters.isEmpty {
            running -= 1
        } else {
            waiters.removeFirst().resume()
        }
    }
}
