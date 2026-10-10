import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalsTabModel {
    public private(set) var state: LoadState<[Components.Schemas.MyCabal]> = .idle
    public private(set) var standings: [String: PortfolioSummary.Row] = [:]
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private var generation = 0
    private var settled = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient) {
        self.api = api
    }

    public func load(withStandings: Bool = false) async {
        generation += 1
        let issued = generation
        if case .loaded = state {} else { state = .loading }
        async let read = withStandings ? readStandings() : nil
        do {
            let cabals = try await api.read { try await $0.getMyCabals().ok.body.json }
            let rows = await read
            guard issued > settled else { return }
            settled = issued
            if let rows { standings = rows }
            state = .loaded(cabals)
            lastError = nil
        } catch {
            if Task.isCancelled { return }
            guard issued > settled else { return }
            settled = issued
            let error = APIError(error)
            if BackgroundRefresh.isActive, case .loaded = state { return }
            lastError = error
            failureTick += 1
            if case .loaded = state { return }
            state = .failed(error)
        }
    }

    private func readStandings() async -> [String: PortfolioSummary.Row]? {
        guard let portfolio = try? await api.read({ try await $0.getMyPortfolio().ok.body.json }) else { return nil }
        return Dictionary(
            PortfolioSummary(portfolio).rows.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
    }

    public func observe(hints: any HintSource) async {
        await refresher.observe(hints.hints(matching: .user(what: "cabals")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
