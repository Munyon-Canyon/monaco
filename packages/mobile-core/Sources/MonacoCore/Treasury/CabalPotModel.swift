import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalPotModel {
    public private(set) var state: LoadState<CabalPotSummary> = .idle
    public private(set) var toast: String?
    public let cabalID: String

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public var summary: CabalPotSummary? {
        if case .loaded(let summary) = state { return summary }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if summary == nil { state = .loading }
        do {
            let pot = try await api.read { [cabalID] client in
                try await client.getCabalPot(path: .init(id: cabalID)).ok.body.json
            }
            guard issued == generation else { return }
            state = .loaded(CabalPotSummary(pot))
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
            hints.hints(matching: .cabal(id: cabalID, what: "activity_changed")),
            hints.hints(matching: .global(what: "prices_updated")),
            hints.hints(matching: .user(what: CashOutJobWatcher.changedHint)),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }
}
