import MonacoAPI
import Observation

@Observable
@MainActor
public final class PortfolioModel {
    public private(set) var state: LoadState<PortfolioSummary> = .idle
    public private(set) var toast: String?

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public var summary: PortfolioSummary? {
        if case .loaded(let summary) = state { return summary }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if summary == nil { state = .loading }
        do {
            let portfolio = try await api.read { try await $0.getMyPortfolio().ok.body.json }
            guard issued == generation else { return }
            state = .loaded(PortfolioSummary(portfolio))
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            let error = APIError(error)
            if summary == nil {
                state = .failed(error)
            } else {
                toast = ToastCopy.message(for: error)
            }
        }
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .user(what: nil)),
            hints.hints(matching: .global(what: "leaderboards_updated")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }
}
