import Foundation
import MonacoAPI

public enum PreIpoEligibility: Equatable, Sendable {
    case eligible
    case researchOnly

    public init(tradable: Bool) {
        self = tradable ? .eligible : .researchOnly
    }

    public var title: String {
        self == .eligible ? PreIpoCopy.eligible : PreIpoCopy.researchOnly
    }

    public var detail: String {
        self == .eligible ? PreIpoCopy.eligibleDetail : PreIpoCopy.researchOnlyDetail
    }

    public var allowsBuy: Bool { self == .eligible }
}

public enum PreIpoChartRange: String, CaseIterable, Sendable {
    case oneYear
    case threeYears
    case all

    public var label: String {
        switch self {
        case .oneYear: PreIpoCopy.rangeOneYear
        case .threeYears: PreIpoCopy.rangeThreeYears
        case .all: PreIpoCopy.rangeAll
        }
    }

    var years: Int? {
        switch self {
        case .oneYear: 1
        case .threeYears: 3
        case .all: nil
        }
    }
}

public struct PreIpoStep: Equatable, Sendable, Identifiable {
    public let date: Date
    public let priceMicros: Int64
    public let isEstimate: Bool
    public let dateLabel: String
    public let priceLabel: String
    public let spokenLabel: String

    public var id: String { "\(isEstimate ? "estimate" : "round")-\(Int64(date.timeIntervalSince1970))" }
}

public struct PreIpoDetail: Equatable, Sendable {
    public struct Profile: Equatable, Sendable {
        public let imageURL: URL?
        public let estimateText: String
        public let estimateCaption: String
        public let ledger: [PreIpoCopy.LedgerLine]
        public let fillLine: String
        public let steps: [PreIpoStep]
        public let countLine: String
        public let statsTop: [AssetStatCell]
        public let statsBottom: [AssetStatCell]

        public func steps(in range: PreIpoChartRange, now: Date) -> [PreIpoStep] {
            guard let years = range.years else { return steps }
            var calendar = Calendar(identifier: .gregorian)
            calendar.timeZone = PreIpoDates.utc
            guard let cutoff = calendar.date(byAdding: .year, value: -years, to: now) else { return steps }
            return steps.filter { $0.isEstimate || $0.date >= cutoff }
        }
    }

    public let eligibility: PreIpoEligibility
    public let profile: Profile?

    public init?(_ detail: Components.Schemas.AssetDetail, locale: Locale = .autoupdatingCurrent) {
        guard detail.kind == .preIpo else { return nil }
        eligibility = PreIpoEligibility(tradable: detail.tradable)
        profile = detail.preIpo.flatMap { Profile(payload: $0, locale: locale) }
    }
}

extension PreIpoDetail.Profile {
    fileprivate init?(payload: Components.Schemas.AssetDetail.PreIpoPayload, locale: Locale) {
        let dates = PreIpoDates(locale: locale)
        guard let estimateDate = dates.parse(payload.estimate.asOf) else { return nil }
        let estimatePrice = UsdAmountFormatter.format(micros: payload.estimate.priceMicros)
        let rounds = payload.rounds.compactMap { round -> PreIpoStep? in
            dates.parse(round.on).map {
                PreIpoStep(
                    date: $0, priceMicros: round.priceMicros, isEstimate: false,
                    dateLabel: dates.monthYearShort($0), priceLabel: PreIpoDates.wholeDollars(round.priceMicros),
                    spokenLabel: "\(dates.monthYearWide($0)) round, \(PreIpoDates.spoken(round.priceMicros))")
            }
        }.sorted { $0.date < $1.date }
        let estimateDay = dates.monthDay(estimateDate)
        let estimateStep = PreIpoStep(
            date: estimateDate, priceMicros: payload.estimate.priceMicros, isEstimate: true,
            dateLabel: PreIpoCopy.estimateStepLabel(estimateDay),
            priceLabel: PreIpoDates.wholeDollars(payload.estimate.priceMicros),
            spokenLabel:
                "\(dates.monthDayWide(estimateDate)) secondary estimate, \(PreIpoDates.spoken(payload.estimate.priceMicros))"
        )
        var ledger: [PreIpoCopy.LedgerLine] = []
        if let last = payload.lastRound, let lastDate = dates.parse(last.on) {
            ledger.append(
                PreIpoCopy.ledgerLastRound(
                    on: dates.monthYearShort(lastDate), price: UsdAmountFormatter.format(micros: last.priceMicros)))
        }
        ledger.append(PreIpoCopy.ledgerEstimate(asOf: estimateDay, price: estimatePrice))
        imageURL = payload.imageUrl.flatMap(URL.init(string:)).flatMap { $0.scheme == nil ? nil : $0 }
        estimateText = estimatePrice
        estimateCaption = PreIpoCopy.estimateCaption(asOf: estimateDay)
        self.ledger = ledger
        fillLine =
            payload.sellWindow == .tenderQuarterly
            ? payload.nextTenderOn.flatMap(dates.parse)
                .map { PreIpoCopy.proposalTenderFillLine(dates.monthYear($0)) } ?? PreIpoCopy.proposalFillLine
            : PreIpoCopy.proposalFillLine
        steps = rounds + [estimateStep]
        countLine = PreIpoCopy.roundsCount(rounds: rounds.count, estimates: 1)
        let valuation = AssetStats.abbreviated(payload.valuationMicros)
        let lastRoundText = payload.lastRound.flatMap { dates.parse($0.on) }.map(dates.monthYear)
        statsTop = [
            Self.cell(PreIpoCopy.valuationLabel, valuation),
            Self.cell(PreIpoCopy.lastRoundLabel, lastRoundText),
            Self.cell(PreIpoCopy.minOrderLabel, PreIpoDates.wholeDollars(payload.minOrderMicros)),
        ]
        var bottom = [Self.cell(PreIpoCopy.sellWindowLabel, Self.sellWindow(payload.sellWindow))]
        if let tender = payload.nextTenderOn.flatMap(dates.parse) {
            bottom.append(Self.cell(PreIpoCopy.nextTenderLabel, dates.monthYear(tender)))
        }
        statsBottom = bottom
    }

    private static func cell(_ label: String, _ value: String?) -> AssetStatCell {
        guard let value else { return AssetStatCell(label: label, value: AssetStats.dash, isAvailable: false) }
        return AssetStatCell(label: label, value: value)
    }

    private static func sellWindow(_ wire: Components.Schemas.PreIpoSellWindow) -> String {
        switch wire {
        case .anyTime: PreIpoCopy.sellWindowAnyTime
        case .tenderQuarterly: PreIpoCopy.sellWindowTender
        }
    }
}

struct PreIpoDates {
    static let utc = TimeZone(secondsFromGMT: 0) ?? .gmt

    let locale: Locale

    func parse(_ iso: String) -> Date? {
        let parts = iso.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3 else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = Self.utc
        return calendar.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2]))
    }

    private var style: Date.FormatStyle {
        Date.FormatStyle(locale: locale, calendar: Calendar(identifier: .gregorian), timeZone: Self.utc)
    }

    func monthYearShort(_ date: Date) -> String {
        let month = date.formatted(style.month(.abbreviated))
        let year = date.formatted(style.year(.twoDigits))
        return "\(month) ’\(year)"
    }

    func monthYear(_ date: Date) -> String { date.formatted(style.month(.abbreviated).year()) }

    func monthYearWide(_ date: Date) -> String { date.formatted(style.month(.wide).year()) }

    func monthDay(_ date: Date) -> String { date.formatted(style.month(.abbreviated).day()) }

    func monthDayWide(_ date: Date) -> String { date.formatted(style.month(.wide).day()) }

    static func wholeDollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: micros)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    static func spoken(_ micros: Int64) -> String {
        "\(wholeDollars(micros).dropFirst()) dollars"
    }
}
