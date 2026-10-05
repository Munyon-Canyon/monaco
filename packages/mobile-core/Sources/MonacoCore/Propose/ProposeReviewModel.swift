import Foundation
import MonacoAPI
import Observation

@Observable @MainActor
public final class ProposeReviewModel {
    public struct Row: Equatable, Sendable {
        public let label: String
        public let value: String

        public init(label: String, value: String) {
            self.label = label
            self.value = value
        }
    }

    public private(set) var isSending = false
    public private(set) var errorMessage: String?
    public private(set) var proposalID: String?

    public let cabal: ProposeCabalInfo
    private let service: ProposeService
    private let cabalID: String
    private let draft: ProposalDraft
    private let preview: ProposePreview
    private let trade: ProposeTrade
    private let submission: IdempotentSubmission

    public init(
        service: ProposeService, cabalID: String, cabal: ProposeCabalInfo, draft: ProposalDraft,
        preview: ProposePreview, trade: ProposeTrade, submission: IdempotentSubmission = IdempotentSubmission()
    ) {
        self.service = service
        self.cabalID = cabalID
        self.cabal = cabal
        self.draft = draft
        self.preview = preview
        self.trade = trade
        self.submission = submission
    }

    public var title: String {
        switch trade {
        case .sell(let holding): "Sell \(holding.quantity(of: draft.amount)) of \(holding.ticker)"
        case .buy(let symbol, let kind, _):
            "Buy \(UsdAmountFormatter.format(micros: draft.amount)) of \(AssetSymbolFormatter.display(symbol, kind: kind))"
        }
    }

    public var reasonTitle: String { trade.isSell ? "Why sell" : "Why buy" }

    public var rows: [Row] {
        guard case .sell(let holding) = trade else { return buyRows }
        var rows: [Row] = []
        if let out = preview.quoteOutAmount, out > 0 {
            rows.append(Row(label: "Raises", value: "about \(UsdAmountFormatter.format(micros: out))"))
        }
        rows.append(Row(label: "Cabal keeps", value: holding.quantity(of: holding.tokenAmount - draft.amount)))
        rows.append(Row(label: "Who votes", value: cabal.voters))
        return rows
    }

    private var buyRows: [Row] {
        guard case .buy(_, let kind, _) = trade else { return [] }
        var rows: [Row] = []
        let quantity = quantity
        if let quantity {
            rows.append(
                Row(label: "Cabal gets", value: "about \(TokenQuantityFormatter.label(quantity: quantity, kind: kind))")
            )
        }
        if let price = price(quantity: quantity) {
            let noun = kind == .preIpo ? "a token" : "a share"
            rows.append(Row(label: "Price", value: "about \(UsdAmountFormatter.format(decimal: price)) \(noun)"))
        }
        if let percent = potPercent {
            rows.append(
                Row(label: "Pot", value: "\(percent)% of \(UsdAmountFormatter.format(micros: preview.potValueMicros))"))
        }
        rows.append(Row(label: "Who votes", value: cabal.voters))
        return rows
    }

    public var reason: String? {
        let text: String
        switch draft {
        case .buy(_, _, let thesis), .sell(_, _, let thesis): text = thesis
        }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    public var sendTitle: String { isSending ? "Sending…" : "Send to cabal" }
    public var successToast: String { "Proposal sent to \(cabal.name)" }

    public func send() async {
        guard !isSending else { return }
        isSending = true
        errorMessage = nil
        defer { isSending = false }
        do {
            proposalID = try await service.propose(cabalID: cabalID, draft: draft, submission: submission)
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    private var quantity: Decimal? {
        guard case .buy(_, _, let tokenDecimals) = trade, let out = preview.quoteOutAmount, out > 0 else { return nil }
        return TokenQuantityFormatter.quantity(fromAtomics: String(out), decimals: tokenDecimals)
    }

    private func price(quantity: Decimal?) -> Decimal? {
        guard case .buy(_, let usdcMicros, _) = draft, let quantity, quantity > 0,
            let usd = TokenQuantityFormatter.quantity(fromAtomics: String(usdcMicros), decimals: 6)
        else { return nil }
        return usd / quantity
    }

    private var potPercent: Int64? {
        let pot = preview.potValueMicros
        guard pot > 0, draft.amount <= Int64.max / 100 else { return nil }
        return (draft.amount * 100 + pot / 2) / pot
    }
}
