import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalModel {
    public static let refreshingHints = ["updated", "members"]

    public private(set) var state: LoadState<Components.Schemas.Cabal> = .idle
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public let cabalID: String
    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var generation = 0

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
        let hook = CabalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public var cabal: Components.Schemas.Cabal? {
        if case .loaded(let cabal) = state { return cabal }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {} else { state = .loading }
        do {
            let cabal = try await api.read { [cabalID] client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            guard issued == generation else { return }
            state = .loaded(cabal)
            lastError = nil
        } catch {
            guard issued == generation else { return }
            let error = APIError(error)
            lastError = error
            failureTick += 1
            if case .loaded = state { return }
            state = .failed(error)
        }
    }

    public func observe() async {
        await refresher.observe(Self.refreshingHints.map { hints.hints(matching: .cabal(id: cabalID, what: $0)) })
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}

@MainActor
private final class CabalReloadHook {
    var run: (@MainActor () async -> Void)?
}
