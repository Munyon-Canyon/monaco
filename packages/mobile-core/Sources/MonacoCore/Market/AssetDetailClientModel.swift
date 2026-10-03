import MonacoAPI
import Observation

public struct AssetDetailPresentation: Equatable, Sendable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let kind: AssetKind
    public let priceMicros: Int64?
    public let changeBasisPoints: Int64?
    public let attribution: String
    public let market: MarketAsset
    public let otherListings: [AssetListingPresentation]
    public let isTradable: Bool
}

public typealias AssetListingPresentation = MarketListing

@Observable
@MainActor
public final class AssetDetailClientModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed(APIError)
    }

    public private(set) var phase: Phase = .idle
    public private(set) var detail: AssetDetailPresentation?
    public private(set) var chart: AssetChartSeries?
    public private(set) var chartError: APIError?
    public private(set) var selectedRange: AssetChartRange = .oneDay
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private var chartGeneration = 0
    private let api: APIClient
    private let symbol: String

    public init(api: APIClient, symbol: String) {
        self.api = api
        self.symbol = symbol
    }

    public func load() async {
        if detail == nil { phase = .loading }
        do {
            let response = try await api.read { client in
                try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
            }
            detail = Self.presentation(response)
            phase = .loaded
            lastError = nil
            await loadChart()
        } catch {
            let error = APIError(error)
            lastError = error
            failureTick += 1
            if detail == nil { phase = .failed(error) }
        }
    }

    public func loadChart(range: AssetChartRange = .oneDay) async {
        selectedRange = range
        chartGeneration += 1
        let issued = chartGeneration
        do {
            let response = try await api.read { client in
                try await client.getAssetChart(
                    path: .init(symbol: symbol),
                    query: .init(range: Self.wireRange(range))
                ).ok.body.json
            }
            guard issued == chartGeneration else { return }
            chart = MarketMapping.chart(response)
            chartError = nil
        } catch {
            guard issued == chartGeneration else { return }
            chartError = APIError(error)
        }
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
            changeBasisPoints: detail.changeBps.map(Int64.init),
            attribution: detail.attribution,
            market: market,
            otherListings: mapped.otherListings,
            isTradable: detail.tradable
        )
    }

    private static func wireRange(
        _ range: AssetChartRange
    ) -> Operations.GetAssetChart.Input.Query.RangePayload {
        switch range {
        case .oneDay: ._1d
        case .oneWeek: ._1w
        case .oneMonth: ._1m
        case .threeMonths: ._3m
        case .oneYear: ._1y
        case .all: .all
        }
    }
}
