import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProposalPauseModel {
    public private(set) var isPaused = false
    private let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher

    public init(cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
        let hook = PauseReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public func load() async {
        if let paused = try? await repository.isPaused(cabalID: cabalID) { isPaused = paused }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: "pause_changed")))
    }

    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }
}

@MainActor
private final class PauseReloadHook {
    var run: (@MainActor () async -> Void)?
}
