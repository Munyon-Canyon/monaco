import Foundation
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
    private var logos: [String: URL] = [:]
    private var logoSymbols: Set<String> = []
    private let logoStore: AssetLogoStore?

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource, logoStore: AssetLogoStore? = nil) {
        self.logoStore = logoStore
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
            let summary = CabalPotSummary(pot)
            state = .loaded(summary.withLogos(logos))
            startLogoLookup(for: summary.holdings.map(\.symbol))
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

    private func startLogoLookup(for symbols: [String]) {
        guard let logoStore else { return }
        let missing = symbols.filter { !logoSymbols.contains($0) }
        guard !missing.isEmpty else { return }
        logoSymbols.formUnion(missing)
        Task { [weak self] in
            let found = await logoStore.logos(for: missing)
            guard let self, !found.isEmpty else { return }
            logos.merge(found) { _, new in new }
            if let summary { state = .loaded(summary.withLogos(logos)) }
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
