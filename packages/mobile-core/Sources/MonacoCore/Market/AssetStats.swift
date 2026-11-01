import Foundation

public struct AssetStatCell: Equatable, Sendable {
    public let label: String
    public let value: String
    public let isAvailable: Bool

    public init(label: String, value: String, isAvailable: Bool = true) {
        self.label = label
        self.value = value
        self.isAvailable = isAvailable
    }

    public var spokenValue: String { isAvailable ? value : "not available" }
}

public struct AssetStats: Equatable, Sendable {
    public static let dash = "—"
    public static let marketCapLabel = "Market cap"
    public static let volumeLabel = "Volume · 24h"
    public static let openLabel = "Open"
    public static let yearLowLabel = "52-week low"
    public static let yearHighLabel = "52-week high"
    public static let heldByLabel = "Held by"

    public let marketCapMicros: String?
    public let volume24hMicros: Int64?
    public let openMicros: Int64?
    public let yearLowMicros: Int64?
    public let yearHighMicros: Int64?
    public let heldByCabals: Int?

    public init(
        marketCapMicros: String?, volume24hMicros: Int64?, openMicros: Int64?, yearLowMicros: Int64?,
        yearHighMicros: Int64?, heldByCabals: Int?
    ) {
        self.marketCapMicros = marketCapMicros
        self.volume24hMicros = volume24hMicros
        self.openMicros = openMicros
        self.yearLowMicros = yearLowMicros
        self.yearHighMicros = yearHighMicros
        self.heldByCabals = heldByCabals
    }

    public var topRow: [AssetStatCell] {
        [
            cell(Self.marketCapLabel, marketCapMicros.flatMap(Self.abbreviated)),
            cell(Self.volumeLabel, volume24hMicros.map { UsdAmountFormatter.compact(micros: $0) }),
            cell(Self.openLabel, openMicros.map { UsdAmountFormatter.format(micros: $0) }),
        ]
    }

    public var bottomRow: [AssetStatCell] {
        [
            cell(Self.yearLowLabel, yearLowMicros.map { UsdAmountFormatter.format(micros: $0) }),
            cell(Self.yearHighLabel, yearHighMicros.map { UsdAmountFormatter.format(micros: $0) }),
            cell(Self.heldByLabel, heldByCabals.flatMap(Self.heldBy)),
        ]
    }

    public var cells: [AssetStatCell] { topRow + bottomRow }

    private func cell(_ label: String, _ value: String?) -> AssetStatCell {
        guard let value else { return AssetStatCell(label: label, value: Self.dash, isAvailable: false) }
        return AssetStatCell(label: label, value: value)
    }

    private static func heldBy(_ count: Int) -> String? {
        switch count {
        case ..<1: nil
        case 1: "1 cabal"
        default: "\(count) cabals"
        }
    }

    static func abbreviated(_ micros: String) -> String? {
        guard !micros.isEmpty, micros.allSatisfy({ $0.isASCII && $0.isNumber }) else { return nil }
        let trimmed = micros.drop { $0 == "0" }
        let dollars = String(trimmed.dropLast(6))
        let steps: [(digits: Int, suffix: String)] = [(13, "T"), (10, "B"), (7, "M"), (4, "K")]
        guard let step = steps.first(where: { dollars.count >= $0.digits }) else {
            return "$\(dollars.isEmpty ? "0" : dollars)"
        }
        let wholeCount = dollars.count - (step.digits - 1)
        let keep = wholeCount + 2
        guard keep <= 18, var hundredths = Int(dollars.prefix(keep)) else { return nil }
        if let next = dollars.dropFirst(keep).first, next >= "5" { hundredths += 1 }
        var body = "\(hundredths / 100)"
        let fraction = hundredths % 100
        if fraction > 0 {
            body += fraction % 10 == 0 ? ".\(fraction / 10)" : ".\(fraction < 10 ? "0" : "")\(fraction)"
        }
        return "$\(body)\(step.suffix)"
    }
}

public struct AssetTodayMove: Equatable, Sendable {
    public let changeMicros: Int64
    public let basisPoints: Int64

    public init?(priceMicros: Int64?, basisPoints: Int64?) {
        guard let priceMicros, let basisPoints, basisPoints > -10_000 else { return nil }
        let (scaled, overflow) = priceMicros.multipliedReportingOverflow(by: basisPoints)
        guard !overflow else { return nil }
        let micros = scaled / (10_000 + basisPoints)
        changeMicros = micros - micros % 10_000
        self.basisPoints = basisPoints
    }

    public var isLoss: Bool { changeMicros < 0 || (changeMicros == 0 && basisPoints < 0) }

    public var dollarsText: String {
        if changeMicros == 0 { return UsdAmountFormatter.format(micros: 0) }
        return changeMicros > 0
            ? UsdAmountFormatter.format(signedMicros: changeMicros) : UsdAmountFormatter.format(micros: changeMicros)
    }

    public var percentText: String {
        PercentFormatter.format(basisPoints: basisPoints < 0 ? -basisPoints : basisPoints, signed: false)
    }

    public var line: String { "\(dollarsText) · \(percentText) today" }

    public func speech(priceMicros: Int64?) -> String {
        guard let priceMicros else { return "" }
        return "\(Self.spoken(priceMicros)), \(direction) \(Self.spoken(changeMicros)), \(percentSpoken) percent today"
    }

    private var direction: String { changeMicros < 0 ? "down" : "up" }

    private var percentSpoken: String {
        String(percentText.dropLast())
    }

    static func spoken(_ micros: Int64) -> String {
        let cents = (micros.magnitude / 10_000)
        let dollars = cents / 100
        let remainder = cents % 100
        let unit = dollars == 1 ? "dollar" : "dollars"
        return remainder == 0 ? "\(dollars) \(unit)" : "\(dollars) \(unit) \(remainder)"
    }
}

public struct AssetAfterHoursMove: Equatable, Sendable {
    public let basisPoints: Int64

    public init?(basisPoints: Int64?) {
        guard let basisPoints, basisPoints > -10_000 else { return nil }
        self.basisPoints = basisPoints
    }

    public var isLoss: Bool { basisPoints < 0 }

    public var line: String {
        "After hours \(PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1))"
    }

    public var speech: String {
        let magnitude = PercentFormatter.format(
            basisPoints: basisPoints < 0 ? -basisPoints : basisPoints, signed: false, fractionDigits: 1)
        let direction = basisPoints < 0 ? "down" : "up"
        return "after hours \(direction) \(magnitude.dropLast()) percent"
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
