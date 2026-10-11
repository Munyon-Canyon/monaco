import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalClosedModel {
    public static let closedStatus = "banned"
    public static let message =
        "Monaco closed this cabal. Everyone's slice is being cashed out to their account balance."

    public private(set) var isClosed = false
    public let cabalID: String
    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource, cabalID: String) {
        self.api = api
        self.hints = hints
        self.cabalID = cabalID
    }

    public func load() async {
        generation += 1
        let issued = generation
        do {
            let cabal = try await api.read { [cabalID] client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            guard issued == generation else { return }
            isClosed = cabal.status == Self.closedStatus
        } catch {
            return
        }
    }

    public func observe() async {
        await refresher.observe([hints.hints(matching: .cabal(id: cabalID, what: "activity_changed"))])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
