import Foundation
import MonacoAPI

public struct HoldingTrade: Equatable, Sendable, Identifiable {
    public let id: String
    public let isBuy: Bool
    public let unitsText: String
    public let dateText: String
    public let unitPriceText: String?
    public let totalMicros: Int64

    public var totalText: String { UsdAmountFormatter.format(micros: totalMicros) }

    public var title: String { CabalHoldingCopy.tradeTitle(isBuy: isBuy, units: unitsText) }

    public var priceLine: String? { unitPriceText.map(CabalHoldingCopy.perUnit) }

    public var spoken: String {
        let price = unitPriceText.map { ", \($0) per unit" } ?? ""
        return "\(isBuy ? "Buy" : "Sell"), \(unitsText) units, \(dateText)\(price), total \(totalText)"
    }

    public init(
        id: String, isBuy: Bool, unitsText: String, dateText: String, unitPriceText: String?, totalMicros: Int64
    ) {
        self.id = id
        self.isBuy = isBuy
        self.unitsText = unitsText
        self.dateText = dateText
        self.unitPriceText = unitPriceText
        self.totalMicros = totalMicros
    }

    public init?(_ activity: Components.Schemas.CabalActivity, symbol: String, decimals: Int) {
        guard activity.status == .confirmed, activity.asset?.symbol == symbol,
            activity.kind == .buy || activity.kind == .sell,
            let micros = activity.usdcMicros, let baseUnits = activity.units
        else { return nil }
        let units = Self.decimal(baseUnits) / Self.scale(decimals)
        self.init(
            id: activity.id,
            isBuy: activity.kind == .buy,
            unitsText: Self.unitsText(units),
            dateText: Self.dateText(activity.occurredAt),
            unitPriceText: units > 0
                ? UsdAmountFormatter.format(decimal: Self.decimal(micros) / 1_000_000 / units) : nil,
            totalMicros: micros)
    }

    static func unitsText(_ units: Decimal) -> String {
        var rounded: Decimal = 0
        var source = units
        NSDecimalRound(&rounded, &source, 4, .plain)
        let formatter = NumberFormatter()
        formatter.numberStyle = .decimal
        formatter.minimumFractionDigits = 4
        formatter.maximumFractionDigits = 4
        formatter.usesGroupingSeparator = false
        formatter.locale = Locale(identifier: "en_US_POSIX")
        return formatter.string(from: rounded as NSDecimalNumber) ?? "\(rounded)"
    }

    static func dateText(_ date: Date) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return SharedFormatters.string(
            from: date, pattern: .fixed("MMM dd, yyyy · HH:mm 'UTC'"), locale: Locale(identifier: "en_US_POSIX"),
            calendar: calendar)
    }

    private static func decimal(_ value: Int64) -> Decimal {
        NSDecimalNumber(value: value).decimalValue
    }

    private static func scale(_ decimals: Int) -> Decimal {
        var scale: Decimal = 1
        for _ in 0..<max(decimals, 0) { scale *= 10 }
        return scale
    }
}
