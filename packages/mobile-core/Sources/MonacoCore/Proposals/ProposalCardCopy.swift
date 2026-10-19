import Foundation

public enum ProposalCardCopy {
    public static func pausedCaption(isSell: Bool) -> String {
        "Trading is paused. If this passes, it won't \(isSell ? "sell" : "buy") until trading resumes."
    }

    public static func closes(at expiry: Date, now: Date) -> String {
        ProposalTimeFormatter.closesLabel(expiry: expiry, now: now)
    }

    public struct Action: Equatable, Sendable {
        public let title: String
        public let isPrimary: Bool
    }

    public enum FooterIcon: Equatable, Sendable { case chat, clock, done, warning }

    public enum FooterTone: Equatable, Sendable { case muted, warning, gain, loss }

    public struct Footer: Equatable, Sendable {
        public let icon: FooterIcon
        public let text: String
        public let tone: FooterTone
    }

    public enum NoticeTone: Equatable, Sendable { case info, warn, error }

    public struct Notice: Equatable, Sendable {
        public let tone: NoticeTone
        public let message: String
    }

    public static func button(_ state: ProposalState) -> Action {
        switch state {
        case .open: Action(title: "Review", isPrimary: true)
        case .voted: Action(title: "Review vote", isPrimary: false)
        case .viewOnly: Action(title: "Review", isPrimary: false)
        case .approved, .blocked: Action(title: "View status", isPrimary: false)
        case .executed: Action(title: "View trade", isPrimary: false)
        case .expired, .rejected, .withdrawn, .voided: Action(title: "View decision", isPrimary: false)
        }
    }

    public static func neededLabel(_ count: Int) -> String {
        count == 1 ? "1 yes vote needed" : "\(count) yes votes needed"
    }

    public static func note(
        _ state: ProposalState, needed: Int, expiresAt: Date, statusMessage: String?, now: Date
    ) -> String {
        switch state {
        case .open, .voted, .viewOnly: "\(neededLabel(needed)) · \(closes(at: expiresAt, now: now))"
        case .approved: "The cabal passed this proposal."
        case .executed: "The trade is complete."
        case .expired: "Voting ended without enough yes votes."
        case .blocked: "Execution was blocked. Review the status."
        case .rejected: "The proposal did not pass."
        case .withdrawn: "The proposer withdrew this proposal."
        case .voided: nonEmpty(statusMessage) ?? "Monaco voided this proposal."
        }
    }

    public static func tallyLine(_ tally: ProposalTally) -> String {
        "\(tally.yes) yes · \(tally.no) no · \(max(tally.voters - tally.yes - tally.no, 0)) not voted"
    }

    public static func footer(_ state: ProposalState, tally: ProposalTally) -> Footer {
        switch state {
        case .open, .voted, .viewOnly: Footer(icon: .chat, text: tallyLine(tally), tone: .muted)
        case .approved:
            Footer(icon: .clock, text: "Approved by the cabal · Not completed", tone: .warning)
        case .executed:
            Footer(icon: .done, text: "Trade confirmed · View the original decision", tone: .gain)
        case .blocked:
            Footer(icon: .warning, text: "Decision retained · No completed trade", tone: .loss)
        case .expired, .rejected, .withdrawn, .voided:
            Footer(icon: .clock, text: "Voting closed · No trade placed", tone: .muted)
        }
    }

    public static func rowSubtitle(_ state: ProposalState, proposer: String, statusMessage: String?) -> String {
        switch state {
        case .open, .voted, .viewOnly: "\(proposer) · Voting open"
        case .approved: "Approved · execution pending"
        case .executed: "Trade confirmed"
        case .expired: "Voting ended without enough yes votes"
        case .blocked: "The trade did not go through"
        case .rejected: "The proposal did not pass"
        case .withdrawn: "The proposer withdrew this proposal"
        case .voided: nonEmpty(statusMessage) ?? "Monaco voided this proposal"
        }
    }

    public static func statusNotice(_ state: ProposalState, failureMessage: String?, statusMessage: String?)
        -> Notice?
    {
        switch state {
        case .open, .voted, .viewOnly:
            return nil
        case .approved:
            return Notice(tone: .warn, message: "The vote passed. Execution is pending.")
        case .executed:
            return Notice(tone: .info, message: "The trade is confirmed.")
        case .expired:
            return Notice(tone: .info, message: "Voting closed without enough yes votes. No trade was placed.")
        case .blocked:
            let lead = "The trade did not go through."
            let detail = (nonEmpty(failureMessage) ?? nonEmpty(statusMessage)).map(sentence)
            let message = detail.flatMap { $0 == lead ? nil : "\(lead) \($0)" } ?? lead
            return Notice(tone: .error, message: message)
        case .rejected:
            return Notice(tone: .info, message: "The proposal did not pass.")
        case .withdrawn, .voided:
            let badge = ProposalBadge.hero(state).label
            let detail = nonEmpty(statusMessage).map(sentence)
            return Notice(tone: .info, message: ["\(badge).", detail].compactMap { $0 }.joined(separator: " "))
        }
    }

    private static func nonEmpty(_ text: String?) -> String? {
        guard let text, !text.isEmpty else { return nil }
        return text
    }

    private static func sentence(_ text: String) -> String { text.hasSuffix(".") ? text : text + "." }

    public static func voter(_ name: String, choice: String?) -> String {
        guard let choice else { return "\(name) hasn't voted" }
        return "\(name) voted \(choice)"
    }

    public static func sellAmount(atomics: String, decimals: Int, kind: AssetKind) -> String {
        let unit = kind == .preIpo ? "tokens" : "shares"
        return "\(ProposalShareFormatter.shares(fromAtomics: atomics, decimals: decimals)) \(unit)"
    }

    public static func age(since created: Date, now: Date) -> String {
        let minutes = max(0, Int(now.timeIntervalSince(created) / 60))
        if minutes < 1 { return "now" }
        if minutes < 60 { return "\(minutes)m" }
        if minutes < 1440 { return "\(minutes / 60)h" }
        return "\(minutes / 1440)d"
    }

    public static func ago(since created: Date, now: Date) -> String {
        let label = age(since: created, now: now)
        return label == "now" ? "Just now" : "\(label) ago"
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
        let noun = shares == 1 ? String(unit.dropLast()) : unit
        return "about \(count) \(noun) at \(priceLabel)"
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
