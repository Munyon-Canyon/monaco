import Foundation

public enum ProposalResultCopy {
    public enum Seal: Equatable, Sendable { case passed, failed, neutral }
    public enum FillUnit: Equatable, Sendable {
        case shares
        case ticker(String)
    }
    public enum Action: Equatable, Sendable { case share, proposeAgain }

    public struct TradeRow: Equatable, Sendable {
        public let title: String
        public let subtitle: String
        public let trailing: String?
        public let tone: ProposalStats.Tone
    }

    public struct Row: Equatable, Sendable {
        public let title: String
        public let subtitle: String?
    }

    private static let posix = Locale(identifier: "en_US_POSIX")

    private static func passes(_ status: ProposalStatus) -> Bool {
        status == .passed || status == .executed || status == .executionBlocked
    }

    public static func headline(status: ProposalStatus, tally: ProposalTally) -> String {
        let score = "\(tally.yes)–\(tally.no)"
        switch status {
        case .passed, .executed, .executionBlocked: return "Passed \(score)"
        case .failed: return "Failed \(score)"
        case .expired: return tally.yes + tally.no > 0 ? "Expired \(score)" : "Expired"
        case .withdrawn: return "Withdrawn"
        case .voided: return "Voided by Monaco"
        case .open: return "Voting open"
        }
    }

    public static func seal(status: ProposalStatus) -> Seal {
        if passes(status) { return .passed }
        return status == .failed ? .failed : .neutral
    }

    public static func tallyLine(tally: ProposalTally) -> String {
        "✓ \(tally.yes) · \(tally.yes + tally.no) of \(tally.voters) voted · \(tally.no) ✕"
    }

    public static func units(atomics: Int64, decimals: Int) -> String? {
        guard atomics >= 0, (0...18).contains(decimals) else { return nil }
        let hundredths: Int64
        if decimals <= 2 {
            hundredths = atomics * Int64(pow10(2 - decimals))
        } else {
            let divisor = Int64(pow10(decimals - 2))
            hundredths = (atomics + divisor / 2) / divisor
        }
        let whole = hundredths / 100
        let cents = hundredths % 100
        if cents == 0 { return "\(whole)" }
        if cents % 10 == 0 { return "\(whole).\(cents / 10)" }
        return cents < 10 ? "\(whole).0\(cents)" : "\(whole).\(cents)"
    }

    private static func pow10(_ exponent: Int) -> UInt64 {
        (0..<exponent).reduce(UInt64(1)) { power, _ in power * 10 }
    }

    static func isFilled(_ swap: ProposalSwap?) -> Bool {
        guard let swap else { return false }
        return swap.status == "confirmed" && swap.filledAt != nil && swap.priceMicros != nil
            && swap.tokenAmount != nil && swap.tokenDecimals != nil
    }

    public static func filled(_ fill: ProposalSwap, unit: FillUnit, isSell: Bool = false) -> String? {
        guard isFilled(fill), let price = fill.priceMicros, let amount = fill.tokenAmount,
            let decimals = fill.tokenDecimals, let count = units(atomics: amount, decimals: decimals)
        else { return nil }
        let name: String
        switch unit {
        case .shares: name = "sh"
        case .ticker(let symbol): name = symbol
        }
        return "\(isSell ? "Sold" : "Bought") \(count) \(name) at \(UsdAmountFormatter.format(micros: price))"
    }

    public static func passedSheetTitle(kind: String, symbol: String, assetKind: AssetKind, amountMicros: Int64?)
        -> String
    {
        let verb = kind == "sell" ? "Sell" : "Buy"
        let ticker = AssetSymbolFormatter.display(symbol, kind: assetKind)
        guard let amountMicros else { return "\(verb) \(ticker)" }
        return "\(verb) \(ticker) · \(ShareProposalText.wholeDollars(amountMicros))"
    }

    public static func passedSheetTally(
        tally: ProposalTally, closedAt: Date?, now: Date, calendar: Calendar = .current
    ) -> String {
        let score = "✓ \(tally.yes) · ✕ \(tally.no)"
        guard let closedAt else { return score }
        let sameDay = calendar.isDate(closedAt, inSameDayAs: now)
        let when = sameDay ? clock(closedAt, calendar: calendar) : day(closedAt, calendar: calendar)
        return "\(score) · closed \(when)"
    }

    public static func passedSheetTrade(_ fill: ProposalSwap, symbol: String, assetKind: AssetKind, isSell: Bool)
        -> String?
    {
        let ticker = AssetSymbolFormatter.display(symbol, kind: assetKind)
        return filled(fill, unit: .ticker(ticker), isSell: isSell).map { $0 + "." }
    }

    public static func calledItLine(proposerName: String, isViewer: Bool, voted: Int, voters: Int) -> String {
        "\(ResultCopy.calledIt(isViewer ? "You" : proposerName)) · \(voted) of \(voters) voted"
    }

    public static func caption(
        status: ProposalStatus, tally: ProposalTally, swap: ProposalSwap?, isSell: Bool, pendingUnits: String?,
        statusMessage: String?, calendar: Calendar = .current
    ) -> String? {
        if let swap, isFilled(swap), let price = swap.priceMicros, let at = swap.filledAt {
            return "filled at \(UsdAmountFormatter.format(micros: price)) · \(day(at, calendar: calendar))"
        }
        switch status {
        case .passed, .executed:
            let verb = isSell ? "selling" : "buying"
            return pendingUnits.map { "\(verb) \($0) · in progress" } ?? "in progress"
        case .failed: return "needed \(tally.needed) · no trade"
        case .expired: return "closed before \(tally.needed) yes · no trade"
        case .executionBlocked: return statusMessage
        case .withdrawn, .voided, .open: return nil
        }
    }

    public static func tradeRow(
        swap: ProposalSwap, isSell: Bool, sinceBps: Int64?, calendar: Calendar = .current
    ) -> TradeRow? {
        guard let title = filled(swap, unit: .shares, isSell: isSell), let at = swap.filledAt else { return nil }
        var subtitle = "Filled \(clock(at, calendar: calendar))"
        if let fee = swap.feeMicros { subtitle += " · fee \(UsdAmountFormatter.format(micros: fee))" }
        return TradeRow(
            title: title, subtitle: subtitle, trailing: sinceBps.map(percent), tone: tone(sinceBps))
    }

    public static func pendingTradeRow(swap: ProposalSwap?, isSell: Bool, units: String?, statusMessage: String?)
        -> Row
    {
        let verb = isSell ? "Selling" : "Buying"
        let title = units.map { "\(verb) \($0)" } ?? verb
        if swap?.status == "failed" {
            return Row(title: title, subtitle: swap?.failureMessage ?? "The trade did not go through.")
        }
        if let statusMessage, swap == nil { return Row(title: title, subtitle: statusMessage) }
        return Row(title: title, subtitle: "In progress")
    }

    public static func myRow(myVote: ProposalMyVote?, choice: ProposalBallot?, status: ProposalStatus) -> Row? {
        guard let choice else { return nil }
        let title = "You voted \(choice.label)"
        guard let myVote else { return Row(title: title, subtitle: nil) }
        if myVote.decided, passes(status) {
            return Row(title: title, subtitle: "The \(ProposalVoteCopy.ordinal(myVote.yesOrder)) yes · it passed")
        }
        if myVote.decided, status == .failed {
            return Row(title: title, subtitle: "The \(ProposalVoteCopy.ordinal(myVote.noOrder)) no · it failed")
        }
        return Row(title: title, subtitle: "\(ProposalVoteCopy.ordinal(myVote.order)) of \(myVote.of)")
    }

    public static func notVotedRow(names: [String], status: ProposalStatus) -> String? {
        guard status == .failed || status == .expired, let first = names.first else { return nil }
        switch names.count {
        case 1: return "\(first) did not vote"
        case 2: return "\(first) and \(names[1]) did not vote"
        default: return "\(first), \(names[1]) and \(names.count - 2) more did not vote"
        }
    }

    public static func action(status: ProposalStatus, canPropose: Bool) -> Action? {
        if passes(status) && status != .executionBlocked { return .share }
        if (status == .failed || status == .expired) && canPropose { return .proposeAgain }
        return nil
    }

    private static func tone(_ bps: Int64?) -> ProposalStats.Tone {
        guard let bps else { return .plain }
        return bps > 0 ? .gain : bps < 0 ? .loss : .plain
    }

    private static func percent(_ bps: Int64) -> String {
        let sign = bps > 0 ? "+" : bps < 0 ? "−" : ""
        let tenths = (bps.magnitude + 5) / 10
        let body = tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
        return "\(sign)\(body)%"
    }

    private static func clock(_ date: Date, calendar: Calendar) -> String {
        SharedFormatters.string(from: date, pattern: .fixed("h:mm a"), locale: posix, calendar: calendar)
    }

    private static func day(_ date: Date, calendar: Calendar) -> String {
        SharedFormatters.string(from: date, pattern: .fixed("MMM d"), locale: posix, calendar: calendar)
    }
}
