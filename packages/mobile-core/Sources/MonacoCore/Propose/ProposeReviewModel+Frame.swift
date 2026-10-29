import Foundation

extension ProposeReviewModel {
    public var isPreIpoBuy: Bool { !isSell && assetKind == .preIpo }
    public var showsThesis: Bool { !isPreIpoBuy }

    public var amountLabel: String {
        isSell ? heroAmount : PreIpoDates.wholeDollars(draft.amount)
    }

    public var assetLine: String {
        guard isPreIpoBuy else { return "\(operation) · market order" }
        guard let shares = estimatedShares else { return "Buy at the estimate" }
        return "Buy \(shares.value) sh at the estimate"
    }

    private var estimatePriceMicros: Int64? { preIpo?.steps.last(where: \.isEstimate)?.priceMicros }

    private var estimatedShares: ProposalStats.Cell? {
        estimatePriceMicros.flatMap { ProposalStats.estimatedShares(usdcMicros: draft.amount, estimateMicros: $0) }
    }

    public var cabalLine: String {
        var parts: [String] = []
        if let cash = preview.spendableCashMicros { parts.append("Cash \(PreIpoDates.wholeDollars(cash))") }
        let window = cabal.votingWindowShort
        if isPreIpoBuy {
            if !window.isEmpty { parts.append(window) }
            parts.append("\(toPass) to pass")
        } else if !window.isEmpty {
            parts.append("\(window) to vote")
        }
        return parts.joined(separator: " · ")
    }

    private var toPass: String {
        cabal.voterCount > 0 ? "\(cabal.neededToPass) of \(cabal.voterCount)" : ProposalStats.missing
    }

    public var stats: [ProposalStats.Cell] {
        [
            .init(label: "Shares", value: sharesValue),
            .init(label: "Price", value: priceValue),
            .init(label: "Of treasury", value: ProposalStats.percent(treasuryBps)),
            .init(label: "To pass", value: toPass),
        ]
    }

    public var preIpoLedger: [PreIpoCopy.LedgerLine] { preIpo?.ledger ?? [] }

    private var sharesValue: String {
        if case .sell(let holding) = trade {
            return holding.quantity(of: draft.amount).split(separator: " ").first.map(String.init)
                ?? ProposalStats.missing
        }
        return quantity(preview).map { ProposalStats.sharesText($0, exact: false) } ?? ProposalStats.missing
    }

    private var priceValue: String {
        if case .sell(let holding) = trade { return UsdAmountFormatter.format(micros: holding.priceMicros) }
        return price(quantity: quantity(preview)).map { UsdAmountFormatter.format(decimal: $0) }
            ?? ProposalStats.missing
    }

    private var treasuryBps: Int64? {
        let pot = preview.potValueMicros
        let value = isSell ? (preview.quoteOutAmount ?? 0) : draft.amount
        guard pot > 0, value > 0, value <= Int64.max / 10_000 else { return nil }
        return value * 10_000 / pot
    }
}
