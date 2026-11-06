import Foundation

extension ProposeReviewModel {
    public var isPreIpoBuy: Bool { !isSell && assetKind == .preIpo }
    public var showsThesis: Bool { true }

    public var thesisPrompt: String { "Why \(assetName)?" }
    public var thesisOptionalLabel: String { "Optional" }

    public var thesisCounter: String? {
        ProposeReasonRules.showsCounter(for: thesis) ? "\(thesis.count) / \(ProposeReasonRules.thesisLimit)" : nil
    }

    public var foldedTradeLine: String { "\(operation) \(assetName) · \(amountLabel)" }

    public var amountLabel: String {
        isSell ? heroAmount : PreIpoDates.wholeDollars(draft.micros)
    }

    public var assetLine: String {
        guard isPreIpoBuy else { return "\(operation) at the market price" }
        guard let shares = estimatedShares else { return "Buy at the estimate" }
        return "Buy \(shares.value) sh at the estimate"
    }

    private var estimatePriceMicros: Int64? { preIpo?.steps.last(where: \.isEstimate)?.priceMicros }

    private var estimatedShares: ProposalStats.Cell? {
        estimatePriceMicros.flatMap { ProposalStats.estimatedShares(usdcMicros: draft.micros, estimateMicros: $0) }
    }

    public var cabalLine: String {
        var parts: [String] = []
        if !isSell, let cash = preview.spendableCashMicros { parts.append("\(PreIpoDates.wholeDollars(cash)) cash") }
        let window = cabal.votingWindowShort
        if !window.isEmpty { parts.append("\(window) to vote") }
        if isPreIpoBuy { parts.append("\(toPass) to pass") }
        return parts.joined(separator: " · ")
    }

    private var toPass: String {
        cabal.voterCount > 0 ? "\(cabal.neededToPass) of \(cabal.voterCount)" : ProposalStats.missing
    }

    public var treasuryLine: String? {
        treasuryBps.map { "\(ProposalStats.percent($0)) of treasury" }
    }

    public var factsLeading: String {
        guard let shares = sharesValue else { return "" }
        return "About \(shares) shares" + (priceValue.map { " at \($0)" } ?? "")
    }

    public var factsTrailing: String { cabal.voterCount > 0 ? "\(toPass) to pass" : "" }

    public var factsLine: String { [factsLeading, factsTrailing].filter { !$0.isEmpty }.joined(separator: " · ") }

    public var preIpoLedger: [PreIpoCopy.LedgerLine] { preIpo?.ledger ?? [] }

    private var sharesValue: String? {
        if case .sell(let holding) = trade {
            return holding.quantity(of: draft.amount).split(separator: " ").first.map(String.init)
        }
        return quantity(preview).map { ProposalStats.sharesText($0, exact: true) }
    }

    private var priceValue: String? {
        if case .sell(let holding) = trade { return UsdAmountFormatter.format(micros: holding.priceMicros) }
        return price(quantity: quantity(preview)).map { UsdAmountFormatter.format(decimal: $0) }
    }

    private var treasuryBps: Int64? {
        let pot = preview.potValueMicros
        let value = isSell ? (preview.quoteOutMicros ?? 0) : draft.micros
        guard pot > 0, value > 0, value <= Int64.max / 10_000 else { return nil }
        return value * 10_000 / pot
    }
}
