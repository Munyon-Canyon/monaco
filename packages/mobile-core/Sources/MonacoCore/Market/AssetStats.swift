import Foundation

public struct AssetStats: Equatable, Sendable {
    public let open: Int64?
    public let dayHigh: Int64?
    public let dayLow: Int64?
    public let previousClose: Int64?
    public let yearHigh: Int64?
    public let yearLow: Int64?

    public var isEmpty: Bool {
        [open, dayHigh, dayLow, previousClose, yearHigh, yearLow].allSatisfy { $0 == nil }
    }

    public init(day: AssetChartSeries?, year: AssetChartSeries?, priceMicros: Int64?, changeBasisPoints: Int64?) {
        let dayCandles = (day?.points ?? []).filter(\.hasCandle)
        open = dayCandles.first?.openUsdcMicros
        dayHigh = dayCandles.map(\.highUsdcMicros).max()
        dayLow = dayCandles.map(\.lowUsdcMicros).min()
        let yearCandles = (year?.points ?? []).filter(\.hasCandle)
        yearHigh = yearCandles.map(\.highUsdcMicros).max()
        yearLow = yearCandles.map(\.lowUsdcMicros).min()
        previousClose = Self.previousClose(priceMicros: priceMicros, changeBasisPoints: changeBasisPoints)
    }

    public func yearRangeBasisPoints(priceMicros: Int64?) -> Int64? {
        guard let yearLow, let yearHigh, yearHigh > yearLow, let priceMicros else { return nil }
        let (scaled, overflow) = (priceMicros - yearLow).multipliedReportingOverflow(by: 10_000)
        guard !overflow else { return nil }
        return min(10_000, max(0, scaled / (yearHigh - yearLow)))
    }

    private static func previousClose(priceMicros: Int64?, changeBasisPoints: Int64?) -> Int64? {
        guard let priceMicros, let changeBasisPoints, changeBasisPoints > -10_000 else { return nil }
        let (scaled, overflow) = priceMicros.multipliedReportingOverflow(by: 10_000)
        guard !overflow else { return nil }
        return scaled / (10_000 + changeBasisPoints)
    }
}

public struct AssetCabalPosition: Equatable, Sendable, Identifiable {
    public let cabalID: String
    public let cabalName: String
    public let pictureURL: String?
    public let canVote: Bool
    public let units: String
    public let kind: AssetKind
    public let valueMicros: Int64
    public let pnlMicros: Int64
    public let returnBasisPoints: Int64?

    public var id: String { cabalID }
    public var sharesLabel: String { CabalPotSummary.shares(units, kind: kind) }

    public init(
        cabalID: String, cabalName: String, pictureURL: String?, canVote: Bool,
        units: String, kind: AssetKind, valueMicros: Int64, pnlMicros: Int64, costBasisMicros: Int64
    ) {
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.pictureURL = pictureURL
        self.canVote = canVote
        self.units = units
        self.kind = kind
        self.valueMicros = valueMicros
        self.pnlMicros = pnlMicros
        let (scaled, overflow) = pnlMicros.multipliedReportingOverflow(by: 10_000)
        returnBasisPoints = costBasisMicros > 0 && !overflow ? scaled / costBasisMicros : nil
    }
}
