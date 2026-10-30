import Foundation
import MonacoAPI

public enum HoldingPeriod: String, CaseIterable, Sendable {
    case oneWeek = "1W"
    case oneMonth = "1M"
    case threeMonths = "3M"
    case oneYear = "1Y"
    case all = "All"

    public static let defaultPeriod = HoldingPeriod.oneMonth

    public var label: String { rawValue }

    public var chartRange: LeaderboardRange {
        switch self {
        case .oneWeek: .oneWeek
        case .oneMonth, .threeMonths: .oneMonth
        case .oneYear, .all: .all
        }
    }
}

public struct CabalHolding: Equatable, Sendable {
    public enum Tone: Equatable, Sendable {
        case neutral
        case gain
        case loss
        case missing

        init(signedMicros: Int64) {
            self = signedMicros < 0 ? .loss : .gain
        }
    }

    public struct Stat: Equatable, Sendable, Identifiable {
        public let label: String
        public let value: String
        public let tone: Tone

        public var id: String { label }

        public var spoken: String {
            value == CabalHoldingCopy.missing ? "\(label), not available yet" : "\(label), \(value)"
        }
    }

    public struct Figures: Equatable, Sendable {
        public let averageBuyMicros: Int64?
        public let averageSellMicros: Int64?
        public let realizedMicros: Int64?
        public let series: [HoldingPeriod: ValueCurve]

        public init(
            averageBuyMicros: Int64? = nil, averageSellMicros: Int64? = nil, realizedMicros: Int64? = nil,
            series: [HoldingPeriod: ValueCurve] = [:]
        ) {
            self.averageBuyMicros = averageBuyMicros
            self.averageSellMicros = averageSellMicros
            self.realizedMicros = realizedMicros
            self.series = series
        }

        public static let none = Figures()

        func withAverageBuy(_ micros: Int64?) -> Figures {
            Figures(
                averageBuyMicros: micros ?? averageBuyMicros, averageSellMicros: averageSellMicros,
                realizedMicros: realizedMicros, series: series)
        }
    }

    public let cabalID: String
    public let symbol: String
    public let ticker: String
    public let assetName: String
    public let cabalName: String
    public let assetKind: AssetKind
    public let logoURL: URL?
    public let valueMicros: Int64
    public let costBasisMicros: Int64
    public let pnlMicros: Int64
    public let unitsText: String
    public let staleMinutes: Int?
    public let figures: Figures

    public init(
        cabalID: String, symbol: String, assetName: String, cabalName: String, assetKind: AssetKind = .stock,
        logoURL: URL? = nil, valueMicros: Int64, costBasisMicros: Int64, pnlMicros: Int64, unitsText: String,
        staleMinutes: Int? = nil, figures: Figures = .none
    ) {
        self.cabalID = cabalID
        self.symbol = symbol
        self.ticker = AssetSymbolFormatter.display(symbol, kind: assetKind)
        self.assetName = assetName
        self.cabalName = cabalName
        self.assetKind = assetKind
        self.logoURL = logoURL
        self.valueMicros = valueMicros
        self.costBasisMicros = costBasisMicros
        self.pnlMicros = pnlMicros
        self.unitsText = unitsText
        self.staleMinutes = staleMinutes
        self.figures = figures
    }

    public var isEmpty: Bool { valueMicros == 0 && costBasisMicros == 0 && unitsText == "0" }

    public var title: String { CabalHoldingCopy.title(assetName) }

    public var header: String { CabalHoldingCopy.header(ticker: ticker, cabal: cabalName) }

    public var headerSpoken: String {
        CabalHoldingCopy.headerSpoken(name: assetName, ticker: ticker, cabal: cabalName)
    }

    public var valueText: String { UsdAmountFormatter.format(micros: valueMicros) }

    public var hasSeries: Bool { !figures.series.isEmpty }

    public func curve(for period: HoldingPeriod) -> ValueCurve? { figures.series[period] }

    public func returnMicros(for period: HoldingPeriod) -> Int64 {
        guard let curve = figures.series[period], let first = curve.samples.first, let last = curve.samples.last else {
            return pnlMicros
        }
        return last.value - first.value
    }

    public func returnText(for period: HoldingPeriod) -> String {
        UsdAmountFormatter.format(signedMicros: returnMicros(for: period))
    }

    public func returnTone(for period: HoldingPeriod) -> Tone {
        isEmpty ? .gain : Tone(signedMicros: returnMicros(for: period))
    }

    public func returnLabel(for period: HoldingPeriod) -> String {
        CabalHoldingCopy.pnlLabel(hasSeries ? period.label : CabalHoldingCopy.allTime)
    }

    public var hasMissingFigures: Bool { stats.contains { $0.tone == .missing } }

    public var hasPendingFigures: Bool { figures.averageSellMicros == nil || figures.realizedMicros == nil }

    public var averageBuyText: String {
        figures.averageBuyMicros.map { UsdAmountFormatter.format(micros: $0) } ?? CabalHoldingCopy.missing
    }

    public var averageBuyDetail: String { CabalHoldingCopy.averageBuyDetail(averageBuyText) }

    public var stats: [Stat] {
        [
            Stat(label: CabalHoldingCopy.Stat.units, value: unitsText, tone: .neutral),
            money(CabalHoldingCopy.Stat.averageBuy, figures.averageBuyMicros, signed: false),
            money(CabalHoldingCopy.Stat.averageSell, figures.averageSellMicros, signed: false),
            isEmpty
                ? Stat(label: CabalHoldingCopy.Stat.costBasis, value: CabalHoldingCopy.missing, tone: .missing)
                : Stat(
                    label: CabalHoldingCopy.Stat.costBasis,
                    value: UsdAmountFormatter.format(micros: costBasisMicros), tone: .neutral),
            money(CabalHoldingCopy.Stat.realized, figures.realizedMicros, signed: true),
            isEmpty
                ? Stat(label: CabalHoldingCopy.Stat.unrealized, value: CabalHoldingCopy.missing, tone: .missing)
                : Stat(
                    label: CabalHoldingCopy.Stat.unrealized,
                    value: UsdAmountFormatter.format(signedMicros: pnlMicros), tone: Tone(signedMicros: pnlMicros)),
        ]
    }

    private func money(_ label: String, _ micros: Int64?, signed: Bool) -> Stat {
        guard let micros else { return Stat(label: label, value: CabalHoldingCopy.missing, tone: .missing) }
        let text = signed ? UsdAmountFormatter.format(signedMicros: micros) : UsdAmountFormatter.format(micros: micros)
        return Stat(label: label, value: text, tone: signed ? Tone(signedMicros: micros) : .neutral)
    }
}
