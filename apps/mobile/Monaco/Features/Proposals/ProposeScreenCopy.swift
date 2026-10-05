import MonacoCore
import SwiftUI

enum ProposeScreenCopy {
    static func sellRowDetail(names: [String]) -> String {
        let shown = names.prefix(3).joined(separator: ", ")
        let more = names.count - 3
        guard more > 0 else { return shown }
        return "\(shown) and \(more) more"
    }

    static func cantBuyCaption(name: String) -> String {
        "\(name) · \(ProposeFlowCopy.cantBuy)"
    }

    static let getsRow = "Cabal gets"
    static let potRow = "Pot"
    static let whoVotesRow = "Who votes"
    static let raisesRow = "Raises"
    static let keepsRow = "Cabal keeps"
    static let keepsNothing = "None"

    static func about(_ figure: String) -> String {
        "about \(figure)"
    }

    static func buyHeadline(amount: String, ticker: String) -> String {
        ProposalFeedCopy.buyHeadline(symbol: ticker, amount: amount)
    }

    static func sellHeadline(shares: String, ticker: String) -> String {
        "Sell \(shares) of \(ticker)"
    }

    static func potShare(amountMicros: Int64, potMicros: Int64) -> String? {
        guard potMicros > 0, amountMicros > 0 else { return nil }
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

    static func reasonTitle(isSell: Bool) -> String {
        isSell ? "Why sell" : "Why buy"
    }

    static func keeps(heldAtomics: Int64, soldAtomics: Int64) -> String {
        let left = heldAtomics - soldAtomics
        guard left > 0 else { return keepsNothing }
        return ProposalShareFormatter.sharesLabel(fromAtomics: String(left))
    }

    static let botRulesTitle = "How it works"
    static let botTrades = "It buys and sells stocks for the cabal, only inside its budget."
    static let botAnswers = "Anyone in the cabal can propose pausing or removing it."

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
