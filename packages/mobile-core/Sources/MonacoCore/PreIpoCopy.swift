import Foundation

/// User-facing copy for Tessera pre-IPO surfaces (Stocks tab, detail, propose nudge).
public enum PreIpoCopy {
    public static let sectionTitle = "Pre-IPO"
    public static let chipLabel = "Pre-IPO"
    public static let tradesAroundTheClock = "Trades around the clock"
    public static let privateMarketReference = "Private-market reference"
    public static let referenceUnavailable = "Reference unavailable"
    public static let companyValueCaption = "Company value"
    public static let aboutCardTitle = "About pre-IPO tokens"
    public static let disclosure =
        "Pre-IPO tokens track a private company before it lists. They pay out if the company goes public or is sold. You can sell anytime here. A 0.2% fee applies when buying and when selling."
    public static let termsLinkTitle = "Terms at tessera.pe"
    public static let termsURL = "https://terms.tessera.pe"
    public static let alsoAvailableFrom = "Also available from"
    public static let chartEmpty = "Price history builds up over time."
    public static let chartEmptyMessage = "The curve draws as the token trades."

    public static let tokenLabelPlural = "tokens"
    public static let tokenLabelSingular = "token"
    public static let tokensRowLabel = "Tokens"

    /// The line under the reference price: the company's value and how old the mark is.
    /// `age` is `RelativeTimeFormatter`'s word: "now", "15m", "3h", or a date. Each reads
    /// differently in a sentence, and "updated now ago" is what gluing them produced.
    public static func referenceCaption(companyValue: String, age: String?) -> String {
        let lead = "\(companyValueCaption) \(companyValue)"
        guard let age = age?.trimmingCharacters(in: .whitespaces), !age.isEmpty else { return lead }
        if age == "now" { return "\(lead) · updated just now" }
        let isElapsed = age.last.map { "mhd".contains($0) } == true && age.dropLast().allSatisfy(\.isNumber)
        return isElapsed ? "\(lead) · updated \(age) ago" : "\(lead) · updated \(age)"
    }

    /// Short chip beside the reference row, e.g. "27% below".
    public static func premiumChip(bps: Int) -> String {
        let pct = abs(bps) / 100
        return bps < 0 ? "\(pct)% below" : "\(pct)% above"
    }

    /// Nudge on propose amount and proposal cards when premium is wide.
    public static func tradingPremiumNudge(bps: Int) -> String {
        let pct = abs(bps) / 100
        let direction = bps < 0 ? "below" : "above"
        return "Trading \(pct)% \(direction) its private-market reference. That gap can widen or close."
    }

    public static func showsPremiumNudge(premiumBps: Int?, assetKind: AssetKind) -> Bool {
        guard let premiumBps else { return false }
        let magnitude = abs(premiumBps)
        switch assetKind {
        case .preIpo: return magnitude >= 1000
        case .stock: return magnitude >= 100
        }
    }

    public struct LedgerLine: Equatable, Sendable {
        public let label: String
        public let value: String

        public init(label: String, value: String) {
            self.label = label
            self.value = value
        }
    }

    public static let privateLine = "Private · priced per round, not live"
    public static let estimateTitle = "Secondary estimate"
    public static let estimateInfoLabel = "About the secondary estimate"
    public static let eligible = "Proposal eligible"
    public static let eligibleDetail = "your cabals can vote to buy"
    public static let researchOnly = "Research only"
    public static let researchOnlyDetail = "not open to proposals yet"
    public static let estimateUnavailable = "Estimate not available yet"
    public static let loadFailed = "Couldn’t load this company"
    public static let retry = "Retry"
    public static let roundStatsTitle = "Round stats"
    public static let roundStatsInfoLabel = "About round stats"
    public static let valuationLabel = "Valuation"
    public static let lastRoundLabel = "Last round"
    public static let minOrderLabel = "Min order"
    public static let sellWindowLabel = "Sell window"
    public static let nextTenderLabel = "Next tender"
    public static let sellWindowTender = "Tender · quarterly"
    public static let sellWindowAnyTime = "Any time"
    public static let roundStatsFootnote = "Secondary estimates update between rounds."
    public static let estimateInfo =
        "A secondary estimate is what shares traded for between funding rounds. It is not a live price."
    public static let roundStatsInfo =
        "Round stats come from the company’s latest funding round. The sell window says when you can sell."
    public static let heldAtEstimate = "at estimate"
    public static let rangeOneYear = "1Y"
    public static let rangeThreeYears = "3Y"
    public static let rangeAll = "All"

    public static func estimateCaption(asOf: String) -> String { "as of \(asOf)" }

    public static func ledgerLastRound(on date: String, price: String) -> LedgerLine {
        LedgerLine(label: "\(lastRoundLabel) · \(date)", value: price)
    }

    public static func ledgerEstimate(asOf date: String, price: String) -> LedgerLine {
        LedgerLine(label: "\(estimateTitle) · \(date)", value: price)
    }

    public static let proposalSubtitle = "Pre-IPO · priced at the secondary estimate"
    public static let proposalFillLine = "fills when it passes"

    public static func proposalEligibilityLine(_ eligibility: PreIpoEligibility) -> String {
        eligibility == .eligible ? "\(eligible) · \(proposalFillLine)" : researchOnly
    }

    public static func roundsCount(rounds: Int, estimates: Int) -> String {
        "\(rounds) \(rounds == 1 ? "round" : "rounds") · \(estimates) \(estimates == 1 ? "estimate" : "estimates")"
    }

    public static func estimateStepLabel(_ date: String) -> String { "\(date) est." }

    public static let auditedStrings: [String] = [
        sectionTitle,
        chipLabel,
        tradesAroundTheClock,
        privateMarketReference,
        referenceUnavailable,
        premiumChip(bps: -2700),
        premiumChip(bps: 2700),
        tradingPremiumNudge(bps: -2700),
        tradingPremiumNudge(bps: 2700),
        companyValueCaption,
        aboutCardTitle,
        disclosure,
        termsLinkTitle,
        alsoAvailableFrom,
        chartEmpty,
        chartEmptyMessage,
        referenceCaption(companyValue: "$32.1B", age: "now"),
        tokenLabelPlural,
        tokenLabelSingular,
        tokensRowLabel,
        privateLine,
        estimateTitle,
        eligible,
        eligibleDetail,
        researchOnly,
        researchOnlyDetail,
        estimateUnavailable,
        loadFailed,
        roundStatsTitle,
        sellWindowTender,
        sellWindowAnyTime,
        roundStatsFootnote,
        estimateInfo,
        roundStatsInfo,
        heldAtEstimate,
        roundsCount(rounds: 3, estimates: 1),
        proposalSubtitle,
        proposalEligibilityLine(.eligible),
        proposalEligibilityLine(.researchOnly),
    ]
}
