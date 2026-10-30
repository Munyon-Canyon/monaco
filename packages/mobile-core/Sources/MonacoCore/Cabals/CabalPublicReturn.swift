import Foundation

public enum CabalPublicReturn: Equatable, Sendable {
    case pending
    case none
    case value(percent: String, pnl: String)

    public static let range = "1M"

    public init(_ curve: ValueCurve?) {
        guard let curve else {
            self = .pending
            return
        }
        guard let first = curve.samples.first, let last = curve.samples.last,
            let firstNav = first.nav, let lastNav = last.nav, firstNav > 0
        else {
            self = .none
            return
        }
        let delta = lastNav - firstNav
        let scaled = delta * 10_000
        let half = firstNav / 2
        let basisPoints = (scaled >= 0 ? scaled + half : scaled - half) / firstNav
        self = .value(
            percent: PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1),
            pnl: UsdAmountFormatter.format(signedMicros: last.pnl - first.pnl))
    }
}
