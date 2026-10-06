import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalsTabModel {
    public private(set) var state: LoadState<[Components.Schemas.MyCabal]> = .idle
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private var generation = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient) {
        self.api = api
    }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {} else { state = .loading }
        do {
            let cabals = try await api.read { try await $0.getMyCabals().ok.body.json }
            guard issued == generation else { return }
            state = .loaded(cabals)
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

    public func observe(hints: any HintSource) async {
        await refresher.observe(hints.hints(matching: .user(what: "cabals")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
