import Foundation
import MonacoAPI
import Observation

public struct AssetDetailPresentation: Equatable, Sendable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let kind: AssetKind
    public let priceMicros: Int64?
    public let priceAsOf: Date?
    public let changeBasisPoints: Int64?
    public let attribution: String
    public let session: MarketSession
    public let market: MarketAsset
    public let otherListings: [AssetListingPresentation]
    public let isTradable: Bool
    public let isQuotable: Bool
}

public typealias AssetListingPresentation = MarketListing

public enum AssetNotice: Equatable, Sendable {
    case stale(minutes: Int)
    case unavailable
    case paused

    public var text: String {
        switch self {
        case .stale(let minutes): MarketStaleness.tokenPriceDelayed(minutes: minutes)
        case .unavailable: "This stock is unavailable for new proposals."
        case .paused: "Trading paused right now."
        }
    }
}

public struct AssetRangeChange: Equatable, Sendable {
    public let basisPoints: Int64?
    public let label: String
}

@Observable
@MainActor
public final class AssetDetailClientModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed(APIError)
    }

    public private(set) var detailPhase: Phase = .idle
    public internal(set) var chartPhase: Phase = .idle
    public private(set) var detail: AssetDetailPresentation?
    public internal(set) var chart: AssetChartSeries?
    public internal(set) var chartError: APIError?
    public internal(set) var selectedRange: AssetChartRange = .oneDay
    public internal(set) var selectedInterval: ChartInterval = .preferred(for: .oneDay)
    public internal(set) var isLoadingOlder = false
    public internal(set) var olderFailureTick = 0
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public private(set) var positions: [AssetCabalPosition] = []
    public private(set) var stats: AssetStats?
    public private(set) var refreshFailed = false
    private(set) var lastLoadedAt: Date?
    var chartGeneration = 0
    var selectionGeneration = 0
    public static let maximumBars = 4_000
    public static let olderPageSize = 500
    let api: APIClient?
    let symbol: String
    let barCap: Int
    private let hints: (any HintSource)?
    let now: @Sendable () -> Date
    private let refresher: HintRefresher

    public var phase: Phase { detailPhase }

    public var heldByVotingCabal: Bool { positions.contains { $0.canVote } }

    public var rangeChange: AssetRangeChange? {
        guard let detail else { return nil }
        return AssetRangeChange(
            basisPoints: chart?.range == selectedRange ? Self.basisPoints(for: chart) : nil,
            label: "\(selectedRange.moveLabel) · \(detail.ticker)"
        )
    }

    public var isShortHistory: Bool {
        guard let chart else { return false }
        return chart.points.count < 2
    }

    public init(
        api: APIClient, symbol: String, hints: any HintSource, barCap: Int = maximumBars,
        now: @escaping @Sendable () -> Date = WallClock.now
    ) {
        self.api = api
        self.symbol = symbol
        self.barCap = barCap
        self.hints = hints
        self.now = now
        let hook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.refresh()
        }
    }

    #if DEBUG
    public init(
        sampleDetail: Components.Schemas.AssetDetail?,
        chart: AssetChartSeries?,
        selectedRange: AssetChartRange = .oneDay,
        selectedInterval: ChartInterval? = nil,
        chartPhase: Phase = .loaded,
        now: @escaping @Sendable () -> Date = WallClock.now
    ) {
        api = nil
        symbol = sampleDetail?.symbol ?? ""
        barCap = Self.maximumBars
        hints = nil
        self.now = now
        refresher = HintRefresher {}
        detail = sampleDetail.map(Self.presentation)
        detailPhase = sampleDetail == nil ? .loading : .loaded
        self.chart = chart
        self.selectedRange = selectedRange
        self.selectedInterval = selectedInterval ?? chart?.interval ?? .default(for: selectedRange)
        self.chartPhase = chartPhase
    }
    #endif

    public func load() async {
        guard await loadDetail() else { return }
        await loadChart(range: selectedRange, interval: selectedInterval)
    }

    @discardableResult
    private func loadDetail() async -> Bool {
        guard let api else { return false }
        if detail == nil { detailPhase = .loading }
        do {
            let response = try await api.read { client in
                try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
            }
            detail = Self.presentation(response)
            detailPhase = .loaded
            lastError = nil
            noteFresh()
            return true
        } catch {
            let error = APIError(error)
            lastError = error
            failureTick += 1
            refreshFailed = detail != nil
            if detail == nil { detailPhase = .failed(error) }
            return false
        }
    }

    public func start() async {
        async let cabalPositions: Void = loadCabalPositions()
        async let year = loadSeries(.oneYear)
        async let drawn: Void = loadChart(range: selectedRange, interval: selectedInterval)
        await loadDetail()
        await drawn
        let day = await loadSeries(.oneDay)
        publishStats(day: day, year: nil)
        publishStats(day: day, year: await year)
        await cabalPositions
    }

    public func loadCabalPositions() async {
        guard let api else { return }
        guard let cabals = try? await api.read({ try await $0.getMyCabals().ok.body.json }) else { return }
        let symbol = symbol
        let reads = await withTaskGroup(of: PotRead.self) { group in
            for cabal in cabals {
                group.addTask {
                    guard
                        let pot = try? await api.read({
                            try await $0.getCabalPot(path: .init(id: cabal.id)).ok.body.json
                        })
                    else { return .failed(cabalID: cabal.id) }
                    guard
                        let holding = pot.holdings.first(where: {
                            $0.symbol.caseInsensitiveCompare(symbol) == .orderedSame
                        })
                    else { return .lacks }
                    return .holds(
                        AssetCabalPosition(
                            cabalID: cabal.id, cabalName: cabal.name, pictureURL: cabal.pictureUrl,
                            canVote: cabal.canVote, units: holding.units, kind: AssetKind(raw: holding.kind.rawValue),
                            valueMicros: holding.valueMicros,
                            pnlMicros: holding.pnlMicros, costBasisMicros: holding.costBasisMicros))
                }
            }
            var results: [PotRead] = []
            for await result in group { results.append(result) }
            return results
        }
        var found: [AssetCabalPosition] = []
        for read in reads {
            switch read {
            case .holds(let position): found.append(position)
            case .failed(let cabalID): found += positions.filter { $0.cabalID == cabalID }
            case .lacks: break
            }
        }
        positions = found.sorted {
            $0.valueMicros == $1.valueMicros ? $0.cabalID < $1.cabalID : $0.valueMicros > $1.valueMicros
        }
    }

    public func loadStats() async {
        publishStats(day: await loadSeries(.oneDay), year: await loadSeries(.oneYear))
    }

    private func loadSeries(_ range: AssetChartRange) async -> AssetChartSeries? {
        guard let api else { return nil }
        return try? await chartSeries(api: api, range: range)
    }

    private func publishStats(day: AssetChartSeries?, year: AssetChartSeries?) {
        guard let detail else { return }
        let built = AssetStats(
            day: day, year: year, priceMicros: detail.priceMicros,
            changeBasisPoints: detail.changeBasisPoints)
        stats = built.isEmpty ? nil : built
    }

    private func chartSeries(api: APIClient, range: AssetChartRange) async throws -> AssetChartSeries {
        let interval = ChartInterval.default(for: range)
        if let chart, chart.range == range, chart.interval == interval || chart.interval == .preferred(for: range) {
            return chart
        }
        let symbol = symbol
        let response = try await api.read { client in
            try await client.getAssetChart(path: .init(symbol: symbol), query: Self.query(range, interval, before: nil))
                .ok.body.json
        }
        return MarketMapping.chart(response)
    }

    private enum PotRead: Sendable {
        case holds(AssetCabalPosition)
        case lacks
        case failed(cabalID: String)
    }

    public func observe() async {
        guard let hints else { return }
        await refresher.observe(hints.hints(matching: .global(what: "prices_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func refresh() async {
        guard let api else { return }
        do {
            let response = try await api.read { client in
                try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
            }
            detail = Self.presentation(response)
            detailPhase = .loaded
            lastError = nil
            noteFresh()
            if chartPhase != .loading { await refreshChart() }
        } catch {
            lastError = APIError(error)
            refreshFailed = detail != nil
            if !BackgroundRefresh.isActive { failureTick += 1 }
        }
    }

    private func noteFresh() {
        lastLoadedAt = now()
        refreshFailed = false
    }

    private static func presentation(_ detail: Components.Schemas.AssetDetail) -> AssetDetailPresentation {
        let mapped = MarketMapping.detail(detail)
        let market = mapped.asset
        return AssetDetailPresentation(
            symbol: market.symbol,
            ticker: market.ticker,
            name: market.name,
            kind: market.kind,
            priceMicros: detail.priceMicros,
            priceAsOf: detail.priceAsOf,
            changeBasisPoints: detail.changeBps.map(Int64.init),
            attribution: detail.attribution,
            session: market.session,
            market: market,
            otherListings: mapped.otherListings,
            isTradable: detail.tradable,
            isQuotable: detail.quotable
        )
    }

    private static func basisPoints(for chart: AssetChartSeries?) -> Int64? {
        guard let first = chart?.baselineUsdcMicros,
            let last = chart?.points.last?.priceUsdcMicros,
            first > 0
        else { return nil }
        let (change, changeOverflow) = last.subtractingReportingOverflow(first)
        let (scaled, scaleOverflow) = change.multipliedReportingOverflow(by: 10_000)
        guard !changeOverflow, !scaleOverflow else { return nil }
        return scaled / first
    }
}

extension AssetChartRange {
    var moveLabel: String {
        switch self {
        case .oneDay: "Past day"
        case .oneWeek: "Past week"
        case .oneMonth: "Past month"
        case .threeMonths: "Past three months"
        case .sixMonths: "Past six months"
        case .oneYear: "Past year"
        case .all: "All time"
        }
    }
}

private final class ReloadHook {
    var run: (@MainActor () async -> Void)?
}
