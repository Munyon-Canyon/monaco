import Foundation
import MonacoAPI
import Observation

public enum CabalHoldingOutcome: Equatable, Sendable {
    case loading
    case loaded(CabalHolding)
    case empty(CabalHolding)
    case failed(APIError)
    case notMember
}

public enum HoldingTrades: Equatable, Sendable {
    case loading
    case loaded([HoldingTrade], hasMore: Bool)
    case failed
}

@Observable
@MainActor
public final class CabalHoldingModel {
    public static let staleAfterMinutes = 10
    public static let pageSize = 100
    public static let pagesPerLoad = 3
    public static let relatedLimit = 2

    public let cabalID: String
    public let symbol: String
    public internal(set) var outcome: CabalHoldingOutcome = .loading
    public internal(set) var trades: HoldingTrades = .loading
    public private(set) var isLoadingMoreTrades = false
    public internal(set) var proposals: [ProposalSummary] = []
    public internal(set) var proposalAsset: ProposalAsset?
    public internal(set) var members: [ProposalMember] = []
    public internal(set) var toast: String?
    public internal(set) var cabalName = ""
    public var period = HoldingPeriod.defaultPeriod

    private let api: APIClient
    private let figures: CabalHolding.Figures
    private let now: @Sendable () -> Date
    private let pagesPerLoad: Int
    private var generation = 0
    private var activity: [Components.Schemas.CabalActivity] = []
    private var cursor: String?
    private var decimals = 8

    public init(
        cabalID: String, symbol: String, api: APIClient, figures: CabalHolding.Figures = .none,
        pagesPerLoad: Int = CabalHoldingModel.pagesPerLoad, now: @escaping @Sendable () -> Date
    ) {
        self.cabalID = cabalID
        self.symbol = symbol
        self.api = api
        self.figures = figures
        self.now = now
        self.pagesPerLoad = pagesPerLoad
    }

    public var holding: CabalHolding? {
        switch outcome {
        case .loaded(let holding), .empty(let holding): holding
        case .loading, .failed, .notMember: nil
        }
    }

    public var navigationTitle: String {
        holding?.title ?? CabalHoldingCopy.fallbackTitle
    }

    public func load() async {
        generation += 1
        let issued = generation
        let hadHolding = holding != nil
        if !hadHolding {
            outcome = .loading
            trades = .loading
        }
        async let position = fetchPosition()
        async let performance = fetchPerformance()
        async let history = fetchTrades(cursor: nil)
        async let related = fetchRelated()
        let (positionResult, performanceResult, historyResult, relatedResult) = await (
            position, performance, history, related
        )
        guard issued == generation, !Task.isCancelled else { return }
        switch positionResult {
        case .success(let fetched):
            decimals = fetched.asset?.decimals ?? decimals
            proposalAsset = fetched.asset
            cabalName = fetched.cabalName
            outcome = assemble(fetched, performance: performanceResult)
        case .failure(let error):
            fail(error, hadHolding: hadHolding)
        }
        apply(historyResult, replacing: true)
        proposals = relatedResult.proposals
        members = relatedResult.members
        if let holding { outcome = outcome(for: holding) }
    }

    public func loadMoreTrades() async {
        guard !isLoadingMoreTrades, cursor != nil else { return }
        isLoadingMoreTrades = true
        defer { isLoadingMoreTrades = false }
        apply(await fetchTrades(cursor: cursor), replacing: false)
    }

    public func retryTrades() async {
        trades = .loading
        apply(await fetchTrades(cursor: nil), replacing: true)
    }

    public func dismissToast() {
        toast = nil
    }

    private func outcome(for holding: CabalHolding) -> CabalHoldingOutcome {
        let hasTrades: Bool
        if case .loaded(let rows, _) = trades { hasTrades = !rows.isEmpty } else { hasTrades = false }
        return holding.isEmpty && !hasTrades ? .empty(holding) : .loaded(holding)
    }

    private func fail(_ error: APIError, hadHolding: Bool) {
        if Self.isNotMember(error) {
            outcome = .notMember
            Task { await resolveCabalName() }
        } else if hadHolding {
            if !BackgroundRefresh.isActive { toast = CabalHoldingCopy.refreshFailed }
        } else {
            outcome = .failed(error)
        }
    }

    private func resolveCabalName() async {
        let (cabalID, api) = (cabalID, api)
        let name = try? await api.read { client in
            try await client.getCabal(path: .init(id: cabalID)).ok.body.json.name
        }
        cabalName = name ?? ""
    }

    private func apply(_ result: Result<TradePage, APIError>, replacing: Bool) {
        switch result {
        case .success(let page):
            activity = replacing ? page.items : activity + page.items
            cursor = page.cursor
            trades = .loaded(
                activity.compactMap { HoldingTrade($0, symbol: symbol, decimals: decimals) },
                hasMore: page.cursor != nil)
        case .failure(let error):
            if Self.isNotMember(error) { return }
            if case .loaded = trades, !replacing { return }
            trades = .failed
        }
    }

    private func assemble(_ fetched: Position, performance: HoldingPerformance?) -> CabalHoldingOutcome {
        let held = fetched.pot.holdings.first { $0.symbol == symbol }
        let kind = held.map { AssetKind(raw: $0.kind.rawValue) } ?? fetched.asset?.kind ?? .stock
        let ticker = AssetSymbolFormatter.display(symbol, kind: kind)
        let name = held?.displayName ?? fetched.asset?.displayName ?? ticker
        let base = performance.map { figures.withPerformance($0) } ?? figures
        let minutes = Int(now().timeIntervalSince(fetched.pot.pricesAsOf) / 60)
        let holding = CabalHolding(
            cabalID: cabalID, symbol: symbol, assetName: name, cabalName: fetched.cabalName, assetKind: kind,
            logoURL: fetched.asset?.logoURL,
            valueMicros: held?.valueMicros ?? 0, costBasisMicros: held?.costBasisMicros ?? 0,
            pnlMicros: held?.pnlMicros ?? 0,
            unitsText: held?.units ?? "0",
            staleMinutes: held != nil && minutes >= Self.staleAfterMinutes ? minutes : nil,
            figures: base.withAverageBuy(held?.avgEntryMicros.flatMap { Int64($0) }))
        return outcome(for: holding)
    }

    private struct Position: Sendable {
        let pot: Components.Schemas.CabalPot
        let cabalName: String
        let asset: ProposalAsset?
    }

    private func fetchPerformance() async -> HoldingPerformance? {
        let (cabalID, symbol, api) = (cabalID, symbol, api)
        let fetched = try? await api.read { client in
            try await client.getHoldingPerformance(path: .init(id: cabalID, symbol: symbol)).ok.body.json
        }
        return fetched.map(HoldingPerformance.init)
    }

    private struct TradePage: Sendable {
        let items: [Components.Schemas.CabalActivity]
        let cursor: String?
    }

    private struct Related: Sendable {
        let proposals: [ProposalSummary]
        let members: [ProposalMember]
    }

    private func fetchPosition() async -> Result<Position, APIError> {
        let (cabalID, symbol, api) = (cabalID, symbol, api)
        async let asset = try? ProposalsRepository(api: api).asset(symbol: symbol)
        do {
            async let pot = api.read { client in
                try await client.getCabalPot(path: .init(id: cabalID)).ok.body.json
            }
            async let cabalName = api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json.name
            }
            return .success(Position(pot: try await pot, cabalName: try await cabalName, asset: await asset))
        } catch {
            return .failure(APIError(error))
        }
    }

    private func fetchTrades(cursor start: String?) async -> Result<TradePage, APIError> {
        let (cabalID, symbol, api) = (cabalID, symbol, api)
        var items: [Components.Schemas.CabalActivity] = []
        var matches = 0
        var cursor = start
        do {
            for _ in 0..<pagesPerLoad {
                let page = try await api.read { [cursor] client in
                    try await client.getCabalActivity(
                        path: .init(id: cabalID), query: .init(limit: Self.pageSize, cursor: cursor)
                    ).ok.body.json
                }
                items += page.items
                matches += page.items.filter { Self.isTrade($0, of: symbol) }.count
                cursor = page.nextCursor
                if cursor == nil || matches >= Self.pageSize { break }
            }
            return .success(TradePage(items: items, cursor: cursor))
        } catch {
            return .failure(APIError(error))
        }
    }

    private func fetchRelated() async -> Related {
        let (cabalID, symbol, api) = (cabalID, symbol, api)
        let repository = ProposalsRepository(api: api)
        async let open = try? repository.list(cabalID: cabalID, filter: .open, cursor: nil)
        async let members = try? repository.members(cabalID: cabalID)
        let proposals = (await open?.items ?? []).filter { $0.symbol == symbol }.prefix(Self.relatedLimit)
        return Related(proposals: Array(proposals), members: await members ?? [])
    }

    private nonisolated static func isTrade(_ activity: Components.Schemas.CabalActivity, of symbol: String) -> Bool {
        activity.status == .confirmed && activity.asset?.symbol == symbol
            && (activity.kind == .buy || activity.kind == .sell)
    }

    private nonisolated static func isNotMember(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.notCabalMember)
    }
}
