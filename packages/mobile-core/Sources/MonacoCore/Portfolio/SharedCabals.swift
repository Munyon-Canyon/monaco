import MonacoAPI
import Observation

public struct SharedCabalsSummary: Equatable, Sendable {
    public struct Row: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let pictureURL: String?
        public let potValueMicros: Int64
        public let potValue: String
        public let returnBps: Int64?
        public let returnText: String

        init(_ shared: Components.Schemas.SharedCabal) {
            id = shared.cabal.id
            name = shared.cabal.name
            pictureURL = shared.cabal.pictureUrl
            potValueMicros = shared.valueMicros
            potValue = UsdAmountFormatter.format(micros: shared.valueMicros)
            returnBps = shared.returnBps
            returnText = PortfolioSummary.percent(shared.returnBps)
        }
    }

    public let rows: [Row]

    public var isEmpty: Bool { rows.isEmpty }

    public init(_ shared: Components.Schemas.SharedCabals) {
        rows = shared.cabals.map(Row.init)
    }

    public static func emptyLine(displayName: String?) -> String {
        let name = displayName.flatMap { $0.isEmpty ? nil : $0 } ?? "this person"
        return "You and \(name) aren't in a cabal together yet."
    }
}

@Observable
@MainActor
public final class SharedCabalsModel {
    public private(set) var state: LoadState<SharedCabalsSummary> = .idle
    public private(set) var toast: String?
    public let userID: String

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(userID: String, api: APIClient, hints: any HintSource) {
        self.userID = userID
        self.api = api
        self.hints = hints
    }

    public var summary: SharedCabalsSummary? {
        if case .loaded(let summary) = state { return summary }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if summary == nil { state = .loading }
        do {
            let shared = try await api.read { [userID] client in
                try await client.getSharedCabals(path: .init(id: userID)).ok.body.json
            }
            guard issued == generation else { return }
            state = .loaded(SharedCabalsSummary(shared))
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
        await refresher.observe(hints.hints(matching: .global(what: "leaderboards_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }
}
