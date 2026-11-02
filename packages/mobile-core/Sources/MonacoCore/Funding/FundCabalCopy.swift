import Foundation

public enum FundCabalCopy {
    public static let enterAnAmount = "Enter an amount"
    public static let funding = "Funding…"

    public static func percent(bps: Int32) -> String {
        let value = Int(max(bps, 0))
        if value < 1_000 {
            let tenths = (value + 5) / 10
            if tenths < 100 {
                return tenths % 10 == 0 ? "\(tenths / 10)%" : "\(tenths / 10).\(tenths % 10)%"
            }
        }
        return "\((value + 50) / 100)%"
    }

    public static func amount(micros: Int64) -> String {
        let formatted = UsdAmountFormatter.format(micros: micros)
        return micros % 1_000_000 == 0 ? String(formatted.dropLast(3)) : formatted
    }

    public static let checkAgain = "Check again"

    public static func pendingBlocked(micros: Int64) -> String {
        "Your \(amount(micros: micros)) fund may still be going through. "
            + "Check it again before funding a different amount."
    }

    public static let fromCash = "From your cash"

    public static func shareLine(now: Int32, after: Int32? = nil) -> String {
        guard let after else { return "From your cash · your share \(percent(bps: now))" }
        return "From your cash · your share \(percent(bps: now)) → \(percent(bps: after))"
    }

    public static func limitLine(availableMicros: Int64, overMicros: Int64) -> String {
        "\(UsdAmountFormatter.format(flooredMicros: availableMicros)) available · "
            + "\(UsdAmountFormatter.format(micros: overMicros)) over"
    }

    public static func slotHint(availableMicros: Int64) -> String {
        "Up to \(UsdAmountFormatter.format(flooredMicros: availableMicros))"
    }

    public static func belowMinimum(minMicros: Int64) -> String {
        "Minimum \(amount(micros: minMicros))"
    }

    public static func paused(cabalName: String) -> String {
        "\(cabalName) is paused. Funding opens when it resumes."
    }

    public static func button(micros: Int64) -> String {
        "Fund \(amount(micros: micros))"
    }

    public static func settledLine(now: Int32?) -> String {
        guard let now else { return "Added just now" }
        return "Added just now · your share is now \(percent(bps: now))"
    }

    public static func check(micros: Int64, cabalName: String) -> String {
        "\(amount(micros: micros)) to \(cabalName)"
    }
}
