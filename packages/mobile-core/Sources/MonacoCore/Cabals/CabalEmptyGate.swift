import MonacoAPI
import Observation

public enum CabalEmptyGate {
    public enum History: Equatable, Sendable {
        case unknown
        case none
        case some
    }

    public static func isOn(cabal: Components.Schemas.Cabal?, pot: CabalPotSummary?, history: History) -> Bool {
        guard let cabal, cabal.status == "active", cabal.me != nil, cabal.openProposalCount == 0 else { return false }
        guard history == .none, let pot, pot.state != .zero else { return false }
        return true
    }

    public static func headline() -> String { "Quiet in here." }

    public static func body(name: String, pot: String) -> String {
        let whole = pot.hasSuffix(".00") ? String(pot.dropLast(3)) : pot
        return "\(CabalHeaderCopy.shortName(name)) has \(whole) and no plan yet. The first proposal starts it."
    }
}

@Observable
@MainActor
public final class CabalEmptyModel {
    public private(set) var history = CabalEmptyGate.History.unknown
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

    public func isOn(cabal: Components.Schemas.Cabal?, pot: CabalPotSummary?) -> Bool {
        CabalEmptyGate.isOn(cabal: cabal, pot: pot, history: history)
    }

    public func load() async {
        generation += 1
        let issued = generation
        do {
            let list = try await api.read { [cabalID] client in
                try await client.getCabalProposals(path: .init(id: cabalID), query: .init(filter: .closed, limit: 1))
                    .ok.body.json
            }
            guard issued == generation else { return }
            history = list.proposals.isEmpty ? .none : .some
        } catch {
            return
        }
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_created")),
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
