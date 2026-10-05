import MonacoCore
import SwiftUI

/// Words the propose screens add to `ProposeFlowCopy`, which lives in MonacoCore and is shared with
/// the proposal screens. Kept in one place and audited against `MainFlowCopyAudit` by
/// `ProposeRedesignTests`, for the same reason that table is.
enum ProposeScreenCopy {
    /// The sell row's second line: what the cabal could sell, by name. Three names at most, then
    /// how many more, so a long pot never silently drops a holding from the sentence.
    static func sellRowDetail(names: [String]) -> String {
        let shown = names.prefix(3).joined(separator: ", ")
        let more = names.count - 3
        guard more > 0 else { return shown }
        return "\(shown) and \(more) more"
    }

    // MARK: Pick a stock

    /// A stock that cannot be bought keeps its name and says why, in place of the name alone.
    static func cantBuyCaption(name: String) -> String {
        "\(name) · \(ProposeFlowCopy.cantBuy)"
    }

    // MARK: Receipt

    static let getsRow = "Cabal gets"
    static let potRow = "Pot"
    static let whoVotesRow = "Who votes"
    static let raisesRow = "Raises"
    static let keepsRow = "Cabal keeps"
    static let keepsNothing = "None"

    /// An estimate from the price check, marked as one: "about 0.108 shares".
    static func about(_ figure: String) -> String {
        "about \(figure)"
    }

    /// "Buy $25.00 of AAPL": the exact thing the cabal votes on, in dollars.
    static func buyHeadline(amount: String, ticker: String) -> String {
        ProposalFeedCopy.buyHeadline(symbol: ticker, amount: amount)
    }

    /// "Sell 0.6017 shares of AAPL": a sell is a number of shares, so that is the headline and
    /// the dollars it raises are the estimate underneath.
    static func sellHeadline(shares: String, ticker: String) -> String {
        "Sell \(shares) of \(ticker)"
    }

    /// How much of the pot a buy spends: "4.6% of $548.20". Nil without a pot to measure against.
    static func potShare(amountMicros: Int64, potMicros: Int64) -> String? {
        guard potMicros > 0, amountMicros > 0 else { return nil }
        // To the tenth, and whole numbers without the ".0". Not rounded to whole percents above
        // 10, the way a slice is: 99.6% of the pot is not "100%" on the receipt a vote rests on.
        let tenths = (Double(amountMicros) / Double(potMicros) * 1000).rounded() / 10
        let label: String
        if tenths < 0.1 {
            label = "under 0.1%"
        } else if tenths == tenths.rounded() {
            label = String(format: "%.0f%%", tenths)
        } else {
            label = String(format: "%.1f%%", tenths)
        }
        return "\(label) of \(UsdAmountFormatter.format(micros: potMicros))"
    }

    /// The reason's header on a receipt: the words the proposal's own screen heads it with.
    static func reasonTitle(isSell: Bool) -> String {
        isSell ? "Why sell" : "Why buy"
    }

    /// What the cabal still holds after a sell, in shares; "None" when the sell takes all of it.
    static func keeps(heldAtomics: Int64, soldAtomics: Int64) -> String {
        let left = heldAtomics - soldAtomics
        guard left > 0 else { return keepsNothing }
        return ProposalShareFormatter.sharesLabel(fromAtomics: String(left))
    }

    // MARK: Trading bot

    static let botRulesTitle = "How it works"
    static let botTrades = "It buys and sells stocks for the cabal, only inside its budget."
    static let botAnswers = "Anyone in the cabal can propose pausing or removing it."

    /// Every string above, with representative arguments, for the copy audit.
    static let auditedStrings: [String] = [
        sellRowDetail(names: ["Apple", "Nvidia", "Tesla", "Microsoft"]),
        cantBuyCaption(name: "Amber"),
        getsRow, potRow, whoVotesRow, raisesRow, keepsRow, keepsNothing,
        about("0.108 shares"),
        buyHeadline(amount: "$25.00", ticker: "AAPL"),
        sellHeadline(shares: "0.6017 shares", ticker: "AAPL"),
        potShare(amountMicros: 25_000_000, potMicros: 548_200_000) ?? "",
        keeps(heldAtomics: 120_340_000, soldAtomics: 60_170_000),
        reasonTitle(isSell: false), reasonTitle(isSell: true),
        botRulesTitle, botTrades, botAnswers,
    ]
}
