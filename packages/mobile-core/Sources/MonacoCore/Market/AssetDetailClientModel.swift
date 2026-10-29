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
    public let stats: AssetStats?
    public let preIpo: PreIpoDetail?
    public let marketStats: MarketStatsFacts

    public var todayMove: AssetTodayMove? {
        AssetTodayMove(priceMicros: priceMicros, basisPoints: changeBasisPoints)
    }
}

public typealias AssetListingPresentation = MarketListing

public enum AssetNotice: Equatable, Sendable {
    case unavailable
    case paused

    public var text: String {
        switch self {
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
    public internal(set) var positions: [AssetCabalPosition] = []
    public internal(set) var heldRows: [AssetHeldRowFacts] = []
    public internal(set) var openProposals: [AssetOpenProposalRowFacts] = []
    public private(set) var refreshFailed = false
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
    let feedRefresher: HintRefresher
    let activityRefresher: HintRefresher
    var heldCabalIDs: Set<String> = []
    var heldCabalIDChanges: AsyncStream<Set<String>>.Continuation?

    public var phase: Phase { detailPhase }

    public var stats: AssetStats? { detail?.stats }

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
        let feedHook = ReloadHook()
        let activityHook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        feedRefresher = HintRefresher { await feedHook.run?() }
        activityRefresher = HintRefresher { await activityHook.run?() }
        hook.run = { [weak self] in
            await self?.refresh()
        }
        feedHook.run = { [weak self] in
            await self?.refreshFeed()
        }
        activityHook.run = { [weak self] in
            await self?.loadHeld()
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
        feedRefresher = HintRefresher {}
        activityRefresher = HintRefresher {}
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
        async let inYourCabals: Void = loadInYourCabals()
        async let drawn: Void = loadChart(range: selectedRange, interval: selectedInterval)
        await loadDetail()
        await drawn
        await inYourCabals
    }

    public func observe() async {
        guard let hints else { return }
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await self.refresher.observe(hints.hints(matching: .global(what: "prices_updated"))) }
            group.addTask { await self.feedRefresher.observe(hints.hints(matching: .global(what: "feed"))) }
            group.addTask { await self.observeHeldActivity(hints) }
        }
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
        feedRefresher.setVisible(visible)
        activityRefresher.setVisible(visible)
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
            await loadHeld()
            if chartPhase != .loading, detail?.kind != .preIpo { await refreshChart() }
        } catch {
            lastError = APIError(error)
            refreshFailed = detail != nil
            if !BackgroundRefresh.isActive { failureTick += 1 }
        }
    }

    func refreshFeed() async {
        await loadOpenProposals()
        await loadHeld()
    }

    private func noteFresh() {
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
            isQuotable: detail.quotable,
            stats: market.kind == .preIpo
                ? nil
                : AssetStats(
                    marketCapMicros: detail.stats?.marketCapMicros, volume24hMicros: detail.volume24hMicros,
                    openMicros: detail.stats?.openMicros, yearLowMicros: detail.stats?.yearLowMicros,
                    yearHighMicros: detail.stats?.yearHighMicros, heldByCabals: detail.stats?.heldByCabals),
            preIpo: PreIpoDetail(detail),
            marketStats: MarketStatsFacts(detail)
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
        case .oneHour: "Past hour"
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
