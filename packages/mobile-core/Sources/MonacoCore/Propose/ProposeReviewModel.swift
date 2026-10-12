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
    public private(set) var chain: ProposeChain = .best
    public private(set) var chainPreview: ChainPreview?

    public let cabal: ProposeCabalInfo
    private let service: ProposeService
    private let cabalID: String
    public let draft: ProposalDraft
    private let preview: ProposePreview
    private let trade: ProposeTrade
    private let submission: IdempotentSubmission
    private let buyName: String?
    private let bestChain: AssetBestChain?
    private var chainTask: Task<Void, Never>?

    public init(
        service: ProposeService, cabalID: String, cabal: ProposeCabalInfo, draft: ProposalDraft,
        preview: ProposePreview, trade: ProposeTrade, buyName: String? = nil, bestChain: AssetBestChain? = nil,
        submission: IdempotentSubmission = IdempotentSubmission()
    ) {
        self.buyName = buyName
        self.bestChain = trade.isSell ? nil : bestChain
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
            "Buy \(UsdAmountFormatter.format(micros: draft.amount)) of \(buyName ?? AssetSymbolFormatter.display(symbol, kind: kind))"
        }
    }

    public var isSell: Bool { trade.isSell }
    public var operation: String { isSell ? "Sell" : "Buy" }

    public var heroAmount: String {
        switch trade {
        case .sell(let holding): holding.quantity(of: draft.amount)
        case .buy: UsdAmountFormatter.format(micros: draft.amount)
        }
    }

    public var assetName: String {
        switch trade {
        case .sell(let holding): holding.ticker
        case .buy(let symbol, let kind, _): AssetSymbolFormatter.display(symbol, kind: kind)
        }
    }

    public var bestPrice: BestPrice? {
        bestChain?.bestPrice(forMicros: draft.amount) ?? preview.bestPrice(isSell: isSell)
    }

    public var isEstimate: Bool { bestChain != nil }

    public static let estimateCaption = "Estimate from the price of a $1 buy"

    public var chainOptions: [ProposeChainOption] { ProposeChainOption.options(bestPrice, isEstimate: isEstimate) }

    public func choose(_ chain: ProposeChain) {
        guard !isSending, proposalID == nil else { return }
        self.chain = chain
        chainTask?.cancel()
        chainPreview = nil
        guard let bestChain, let network = chain.network, network != bestChain.network else { return }
        chainPreview = ChainPreview(network: network, state: .loading)
        let draft = draft
        chainTask = Task { [service, cabalID] in
            let state: ChainPreview.State
            do {
                state = .loaded(try await service.preview(cabalID: cabalID, draft: draft, network: network))
            } catch {
                state = .failed
            }
            guard !Task.isCancelled else { return }
            self.chainPreview = ChainPreview(network: network, state: state)
        }
    }

    public var chainProblem: String? {
        guard let chainPreview else { return nil }
        switch chainPreview.state {
        case .loading: return nil
        case .failed: return "Can't get a price on \(BestPrice.chainName(chainPreview.network)) right now"
        case .loaded(let preview): return preview.message(assetName: assetName)
        }
    }

    public var canSend: Bool {
        guard !isSending, proposalID == nil else { return false }
        guard let chainPreview else { return true }
        if case .loading = chainPreview.state { return false }
        return chainProblem == nil
    }

    public var destination: String { "to \(cabal.name)" }
    public var voteCaption: String { "\(cabal.voters) will vote on this proposal." }

    public var reasonTitle: String { trade.isSell ? "Why sell" : "Why buy" }

    public var rows: [Row] {
        guard case .sell(let holding) = trade else { return buyRows }
        var rows: [Row] = []
        if let out = preview.quoteOutAmount, out > 0 {
            rows.append(Row(label: "Raises", value: "about \(UsdAmountFormatter.format(micros: out))"))
        }
        if holding.tokenAmount - draft.amount > 0 {
            rows.append(Row(label: "Cabal keeps", value: holding.quantity(of: holding.tokenAmount - draft.amount)))
        }
        rows.append(Row(label: "Who votes", value: cabal.voters))
        return rows
    }

    private var buyRows: [Row] {
        guard case .buy(_, let kind, _) = trade else { return [] }
        return outcomeRows(kind: kind) + potAndVoterRows
    }

    private func outcomeRows(kind: AssetKind) -> [Row] {
        if let chainPreview {
            guard case .loaded(let pinned) = chainPreview.state else {
                return estimateRows(on: chainPreview.network)
            }
            return quoteRows(pinned, kind: kind)
        }
        if bestChain == nil, let network = chain.network, let bestPrice, let venue = bestPrice.venue(on: network) {
            var rows: [Row] = []
            rows.append(Row(label: "Cabal gets", value: "about \(bestPrice.figure(venue))"))
            if let price = bestPrice.price(of: venue) {
                rows.append(Row(label: "Price", value: "about \(price)"))
            }
            return rows
        }
        return quoteRows(preview, kind: kind)
    }

    private func estimateRows(on network: Components.Schemas.Network) -> [Row] {
        guard let bestPrice, let venue = bestPrice.venue(on: network) else { return [] }
        return [Row(label: "Cabal gets", value: "Estimate: about \(bestPrice.figure(venue))")]
    }

    private func quoteRows(_ preview: ProposePreview, kind: AssetKind) -> [Row] {
        var rows: [Row] = []
        let quantity = quantity(preview)
        if let quantity {
            rows.append(
                Row(label: "Cabal gets", value: "about \(TokenQuantityFormatter.label(quantity: quantity, kind: kind))")
            )
        }
        if let price = price(quantity: quantity) {
            let noun = kind == .preIpo ? "a token" : "a share"
            rows.append(Row(label: "Price", value: "about \(UsdAmountFormatter.format(decimal: price)) \(noun)"))
        }
        return rows
    }

    private var potAndVoterRows: [Row] {
        var rows: [Row] = []
        if let percent = potPercent {
            let pot = UsdAmountFormatter.format(micros: preview.potValueMicros)
            let share = percent == 0 && draft.amount > 0 ? "Less than 1%" : "\(percent)%"
            rows.append(Row(label: "Pot", value: "\(share) of \(pot)"))
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
        guard canSend else { return }
        isSending = true
        errorMessage = nil
        defer { isSending = false }
        do {
            proposalID = try await service.propose(
                cabalID: cabalID, draft: draft, network: chain.network, submission: submission)
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    private func quantity(_ preview: ProposePreview) -> Decimal? {
        guard case .buy(_, _, let tokenDecimals) = trade, let tokenDecimals, let out = preview.quoteOutAmount, out > 0
        else { return nil }
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

public struct ChainPreview: Equatable, Sendable {
    public enum State: Equatable, Sendable {
        case loading
        case loaded(ProposePreview)
        case failed
    }

    public let network: Components.Schemas.Network
    public let state: State
}

@MainActor
public final class ProposeReviewMemory {
    private var held: ProposeReviewModel?

    public init() {}

    public func model(for draft: ProposalDraft) -> ProposeReviewModel? {
        held?.draft == draft ? held : nil
    }

    public func hold(_ model: ProposeReviewModel) -> ProposeReviewModel {
        if let existing = self.model(for: model.draft) { return existing }
        held = model
        return model
    }
}
