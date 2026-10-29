import Foundation
import MonacoAPI
import Observation

public enum CabalEmptyGate {
    public static func isEmpty(
        cabal: Components.Schemas.Cabal?, hasClosedProposal: Bool?, potState: CabalPotSummary.PotState?
    ) -> Bool {
        guard let cabal, cabal.me != nil, cabal.openProposalCount == 0, hasClosedProposal == false,
            let potState, potState != .zero
        else { return false }
        return true
    }

    public static let title = "Quiet in here."

    public static func sentence(cabalName: String, potValue: String) -> String {
        "\(CabalHeaderCopy.shortName(cabalName)) has \(potValue) and no plan yet. The first proposal starts it."
    }

    public static func showsPropose(canPropose: Bool?) -> Bool { canPropose == true }
}

@Observable
@MainActor
public final class CabalEmptyModel {
    public private(set) var hasClosedProposal: Bool?

    private let cabalID: String
    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public func load() async {
        let cabalID = cabalID
        guard
            let page = try? await api.read({ client in
                try await client.getCabalProposals(path: .init(id: cabalID), query: .init(filter: .closed, limit: 1))
                    .ok.body.json
            })
        else { return }
        hasClosedProposal = !page.proposals.isEmpty
    }

    public func observe() async {
        await refresher.observe([hints.hints(matching: .cabal(id: cabalID, what: nil))])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
