import Foundation

public enum ProposalCardCopy {
    public static let pausedCaption = "Trading is paused. If this passes, it won't buy until trading resumes."

    public static func closes(at expiry: Date, now: Date) -> String {
        ProposalTimeFormatter.closesLabel(expiry: expiry, now: now)
    }

    public static func tracker(voted: Int, voters: Int, needed: Int) -> String {
        "\(voted) of \(voters) voted · \(needed) yes to pass"
    }

    public static func voter(_ name: String, choice: String?) -> String {
        guard let choice else { return "\(name) hasn't voted" }
        return "\(name) voted \(choice)"
    }

    public static func viewerVote(_ choice: String?) -> String? {
        choice.map { "✓ You voted \($0)" }
    }

    public static func sellAmount(atomics: String, decimals: Int, kind: AssetKind) -> String {
        let unit = kind == .preIpo ? "tokens" : "shares"
        return "\(ProposalShareFormatter.shares(fromAtomics: atomics, decimals: decimals)) \(unit)"
    }

    public static func age(since created: Date, now: Date) -> String {
        let minutes = max(0, Int(now.timeIntervalSince(created) / 60))
        if minutes < 60 { return "\(minutes)m" }
        if minutes < 1440 { return "\(minutes / 60)h" }
        return "\(minutes / 1440)d"
    }

    public static func expected(
        isSell: Bool, quoteOut: Int64, usdcMicros: Int64?, decimals: Int, kind: AssetKind
    ) -> String? {
        guard quoteOut > 0 else { return nil }
        if isSell { return "about \(UsdAmountFormatter.format(micros: quoteOut))" }
        guard let usdcMicros, usdcMicros > 0 else { return nil }
        let unit = kind == .preIpo ? "tokens" : "shares"
        guard decimals >= 0, decimals <= 18,
            let shares = TokenQuantityFormatter.quantity(fromAtomics: String(quoteOut), decimals: decimals),
            let priceMicros = pricePerShareMicros(usdcMicros: usdcMicros, quoteOut: quoteOut, decimals: decimals)
        else { return nil }
        let priceLabel = UsdAmountFormatter.format(micros: Int64(clamping: priceMicros))
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US")
        formatter.numberStyle = .decimal
        formatter.maximumFractionDigits = 4
        let count = formatter.string(from: shares as NSDecimalNumber) ?? "\(shares)"
        return "about \(count) \(unit) at \(priceLabel)"
    }

    private static func pricePerShareMicros(usdcMicros: Int64, quoteOut: Int64, decimals: Int) -> UInt64? {
        var scale: UInt64 = 1
        for _ in 0..<decimals { scale *= 10 }
        let divisor = UInt64(quoteOut)
        let product = UInt64(usdcMicros).multipliedFullWidth(by: scale)
        let half = divisor / 2
        let (sum, carried) = product.low.addingReportingOverflow(half)
        let numerator = (high: product.high + (carried ? 1 : 0), low: sum)
        guard numerator.high < divisor else { return nil }
        return divisor.dividingFullWidth(numerator).quotient
    }
}
