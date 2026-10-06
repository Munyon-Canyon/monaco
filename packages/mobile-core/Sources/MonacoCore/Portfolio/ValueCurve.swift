import Foundation
import MonacoAPI

public struct CurvePoint: Equatable, Sendable {
    public let at: Date
    public let value: Int64

    public init(at: Date, value: Int64) {
        self.at = at
        self.value = value
    }
}

public struct ValueCurve: Equatable, Sendable {
    public struct Sample: Equatable, Sendable {
        public let at: Date
        public let value: Int64
        public let pnl: Int64
    }

    public struct Readout: Equatable, Sendable {
        public let at: Date
        public let value: String
        public let pnl: String
        public let direction: PortfolioSummary.Direction
    }

    public let samples: [Sample]

    public init(samples: [Sample]) {
        self.samples = samples
    }

    public init(_ history: Components.Schemas.MyPnlHistory) {
        samples = history.points.map { Sample(at: $0.at, value: $0.equityMicros, pnl: $0.pnlMicros) }
    }

    public init(_ history: Components.Schemas.CabalValueHistory) {
        samples = history.points.map { Sample(at: $0.at, value: $0.valueMicros, pnl: $0.pnlMicros) }
    }

    public var points: [CurvePoint] {
        samples.map { CurvePoint(at: $0.at, value: $0.value) }
    }

    public var hasEnoughHistory: Bool { samples.count >= 2 }

    public var direction: PortfolioSummary.Direction {
        guard let first = samples.first, let last = samples.last, samples.count >= 2 else { return .flat }
        if last.pnl > first.pnl { return .up }
        return last.pnl < first.pnl ? .down : .flat
    }

    public func readout(at index: Int? = nil) -> Readout? {
        let sample: Sample?
        if let index {
            sample = samples.indices.contains(index) ? samples[index] : nil
        } else {
            sample = samples.last
        }
        guard let sample else { return nil }
        return Readout(
            at: sample.at,
            value: UsdAmountFormatter.format(micros: sample.value),
            pnl: UsdAmountFormatter.format(signedMicros: sample.pnl),
            direction: PortfolioSummary.Direction(signedMicros: sample.pnl))
    }

    public func nearestIndex(to date: Date) -> Int? {
        samples.indices.min {
            abs(samples[$0].at.timeIntervalSince(date)) < abs(samples[$1].at.timeIntervalSince(date))
        }
    }
}
