import Foundation
import MonacoAPI

extension AssetDetailClientModel {
    public func loadChart(range: AssetChartRange = .oneDay, interval: ChartInterval? = nil) async {
        guard let api else { return }
        let requested = interval ?? ChartInterval.preferred(for: range)
        selectionGeneration += 1
        selectedRange = range
        selectedInterval = requested
        let isDrawn = chart?.range == range && chart?.interval == requested
        chartPhase = isDrawn ? .loaded : .loading
        await readChart(api: api, range: range, interval: requested, replacesChartPhase: !isDrawn)
    }

    func refreshChart() async {
        guard let api else { return }
        await readChart(api: api, range: selectedRange, interval: selectedInterval, replacesChartPhase: false)
    }

    private func readChart(
        api: APIClient, range: AssetChartRange, interval: ChartInterval, replacesChartPhase: Bool
    ) async {
        chartGeneration += 1
        let issued = chartGeneration
        do {
            let response = try await api.read { client in
                try await client.getAssetChart(
                    path: .init(symbol: symbol),
                    query: Self.query(range, interval, before: nil)
                ).ok.body.json
            }
            guard issued == chartGeneration else { return }
            let fresh = MarketMapping.chart(response)
            if let chart, chart.range == fresh.range, chart.interval == fresh.interval {
                self.chart = chart.refreshed(with: fresh)
            } else {
                chart = fresh
            }
            chartError = nil
            chartPhase = .loaded
        } catch {
            guard issued == chartGeneration else { return }
            let error = APIError(error)
            chartError = error
            if replacesChartPhase { chartPhase = .failed(error) }
        }
    }

    public func loadOlder() async {
        guard let api, !isLoadingOlder, let current = chart, let cursor = current.nextBefore,
            current.bars.count < Self.maximumBars
        else { return }
        let issued = selectionGeneration
        isLoadingOlder = true
        defer { isLoadingOlder = false }
        do {
            let response = try await api.read { client in
                try await client.getAssetChart(
                    path: .init(symbol: symbol),
                    query: Self.query(current.range, current.interval, before: cursor, limit: Self.olderPageSize)
                ).ok.body.json
            }
            let older = MarketMapping.chart(response)
            guard issued == selectionGeneration, let latest = chart, latest.range == current.range,
                latest.interval == current.interval, latest.nextBefore == cursor
            else { return }
            chart = latest.prepending(older)
        } catch {
            olderFailureTick += 1
        }
    }

    static func query(
        _ range: AssetChartRange, _ interval: ChartInterval, before: Date?, limit: Int? = nil
    ) -> Operations.GetAssetChart.Input.Query {
        .init(
            range: wireRange(range),
            interval: interval == ChartInterval.default(for: range) ? nil : wireInterval(interval),
            before: before,
            limit: limit
        )
    }

    private static func wireInterval(
        _ interval: ChartInterval
    ) -> Operations.GetAssetChart.Input.Query.IntervalPayload {
        switch interval {
        case .m5: ._5m
        case .m15: ._15m
        case .m30: ._30m
        case .h1: ._1h
        case .h4: ._4h
        case .d1: ._1d
        case .w1: ._1w
        }
    }

    private static func wireRange(
        _ range: AssetChartRange
    ) -> Operations.GetAssetChart.Input.Query.RangePayload {
        switch range {
        case .oneDay: ._1d
        case .oneWeek: ._1w
        case .oneMonth: ._1m
        case .threeMonths: ._3m
        case .sixMonths: ._6m
        case .oneYear: ._1y
        case .all: .all
        }
    }
}
