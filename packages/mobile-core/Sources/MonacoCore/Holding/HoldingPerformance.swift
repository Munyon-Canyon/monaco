import Foundation
import MonacoAPI

struct HoldingPerformance: Equatable, Sendable {
    let averageSellMicros: Int64?
    let realizedMicros: Int64
    let series: [HoldingPeriod: ValueCurve]

    init(_ response: Components.Schemas.HoldingPerformance) {
        averageSellMicros = response.avgSellMicros.flatMap { Int64($0) }
        realizedMicros = response.realizedPnlMicros
        let ranges: [(HoldingPeriod, [Components.Schemas.HoldingPnLPoint])] = [
            (.oneWeek, response.series._1w), (.oneMonth, response.series._1m), (.threeMonths, response.series._3m),
            (.oneYear, response.series._1y), (.all, response.series.all),
        ]
        series = ranges.reduce(into: [:]) { curves, range in
            guard !range.1.isEmpty else { return }
            curves[range.0] = ValueCurve(
                samples: range.1.map { Sample(at: $0.at, value: $0.pnlMicros, pnl: $0.pnlMicros) })
        }
    }
}

private typealias Sample = ValueCurve.Sample
