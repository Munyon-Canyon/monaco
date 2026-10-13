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
        public let nav: Int64?

        public init(at: Date, value: Int64, pnl: Int64, nav: Int64? = nil) {
            self.at = at
            self.value = value
            self.pnl = pnl
            self.nav = nav
        }
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
        samples = history.points.map {
            Sample(at: $0.at, value: $0.valueMicros, pnl: $0.pnlMicros, nav: $0.navPerShareMicros)
        }
    }

    public var points: [CurvePoint] {
        samples.map { CurvePoint(at: $0.at, value: $0.value) }
    }

    public var navPoints: [CurvePoint] {
        samples.compactMap { sample in sample.nav.map { CurvePoint(at: sample.at, value: $0) } }
    }

    private var drawnSamples: [Sample] {
        guard samples.count == 1, let only = samples.first else { return samples }
        return [Sample(at: only.at.addingTimeInterval(-1), value: only.value, pnl: only.pnl, nav: only.nav), only]
    }

    public var drawnPoints: [CurvePoint] { drawnSamples.map { CurvePoint(at: $0.at, value: $0.value) } }

    public var drawnNavPoints: [CurvePoint] {
        drawnSamples.compactMap { sample in sample.nav.map { CurvePoint(at: sample.at, value: $0) } }
    }

    public var hasEnoughHistory: Bool { !samples.isEmpty }

    public var chartBars: [PlottedBar] {
        drawnSamples.map { sample in
            let value = NSDecimalNumber(value: sample.value).dividing(by: NSDecimalNumber(value: 1_000_000)).doubleValue
            return PlottedBar(date: sample.at, open: value, high: value, low: value, close: value)
        }
    }

    public var slotSeconds: Int {
        let gaps = zip(drawnSamples, drawnSamples.dropFirst())
            .map { Int($1.at.timeIntervalSince($0.at).rounded()) }
            .filter { $0 > 0 }
            .sorted()
        guard !gaps.isEmpty else { return 60 }
        return gaps[gaps.count / 2]
    }

    public var direction: PortfolioSummary.Direction {
        guard let first = samples.first, let last = samples.last, samples.count >= 2 else { return .flat }
        if last.pnl > first.pnl { return .up }
        return last.pnl < first.pnl ? .down : .flat
    }

    public func readout(at index: Int? = nil) -> Readout? {
        let sample: Sample?
        if let index {
            let drawn = drawnSamples
            sample = drawn.indices.contains(index) ? drawn[index] : nil
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
        let drawn = drawnSamples
        return drawn.indices.min {
            abs(drawn[$0].at.timeIntervalSince(date)) < abs(drawn[$1].at.timeIntervalSince(date))
        }
    }
}

extension ValueCurve {
    public enum Plot: Sendable { case pnl, nav }

    public struct Change: Equatable, Sendable {
        public let amountText: String?
        public let percentText: String?
        public let direction: PortfolioSummary.Direction
    }

    public func plotted(_ plot: Plot) -> [CurvePoint] {
        drawnSamples.map { CurvePoint(at: $0.at, value: Self.plottedValue($0, plot)) }
    }

    public static func fittedDomain(_ points: [CurvePoint]) -> ClosedRange<Int64> {
        guard let low = points.map(\.value).min(), let high = points.map(\.value).max() else { return 0...1 }
        guard high > low else {
            let half = max(1_000, abs(low) / 200)
            return (low - half)...(low + half)
        }
        let pad = (high - low) * 8 / 100
        return (low - pad)...(high + pad)
    }

    public func change(_ plot: Plot) -> Change? {
        change(plot, to: drawnSamples.count - 1)
    }

    public func change(_ plot: Plot, to index: Int) -> Change? {
        let drawn = drawnSamples
        guard let first = drawn.first, drawn.indices.contains(index) else { return nil }
        let first0 = Self.plottedValue(first, plot)
        let delta = Self.plottedValue(drawn[index], plot) - first0
        let direction = PortfolioSummary.Direction(signedMicros: delta)
        switch plot {
        case .pnl:
            let base = drawn.first { $0.value > 0 }?.value
            return Change(
                amountText: UsdAmountFormatter.format(signedMicros: delta),
                percentText: base.map { Self.percentText(delta, over: $0) },
                direction: direction)
        case .nav:
            return Change(
                amountText: nil,
                percentText: first0 > 0 ? Self.percentText(delta, over: first0) : nil,
                direction: direction)
        }
    }

    private static func plottedValue(_ sample: Sample, _ plot: Plot) -> Int64 {
        switch plot {
        case .pnl: sample.pnl
        case .nav: sample.nav ?? sample.pnl
        }
    }

    private static func percentText(_ delta: Int64, over base: Int64) -> String {
        let scaled = delta * 10_000
        let half = base / 2
        let basisPoints = (scaled >= 0 ? scaled + half : scaled - half) / base
        return PercentFormatter.format(basisPoints: basisPoints, signed: true)
    }
}
