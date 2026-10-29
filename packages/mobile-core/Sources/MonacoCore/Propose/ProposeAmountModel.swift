import Foundation
import MonacoAPI
import Observation

public enum AmountKeypadStateValue: Equatable, Sendable {
    case empty
    case typed
    case invalid(limitLine: String, slot: String)
    case settled(line: String)
}

@Observable @MainActor
public final class ProposeAmountModel {
    public private(set) var amountMicros: Int64 = 0
    public private(set) var preview: ProposePreview?
    public private(set) var cashPreview: ProposePreview?
    public private(set) var cashFailed = false
    public private(set) var previewAmountMicros: Int64?
    public private(set) var previewFailed = false
    public private(set) var isLoading = false
    public private(set) var thesis = ""
    public private(set) var showsThesisRequired = false
    public private(set) var trade: ProposeTrade
    public private(set) var bestChain: AssetBestChain?
    public private(set) var previewBestChain: AssetBestChain?

    private let service: ProposeService
    private let cabalID: String
    private let clock: any Clock<Duration>
    private let bestChains: AssetBestChainStore?
    private var initialAmountMicros: Int64?
    private var previewTask: Task<Void, Never>?

    public init(
        service: ProposeService, cabalID: String, trade: ProposeTrade, clock: any Clock<Duration>,
        bestChains: AssetBestChainStore? = nil, initialAmountMicros: Int64? = nil
    ) {
        self.service = service
        self.cabalID = cabalID
        self.trade = trade
        self.clock = clock
        self.bestChains = bestChains
        self.initialAmountMicros = initialAmountMicros
    }

    public var maxMicros: Int64? {
        switch trade {
        case .buy: cashFigures?.maxBuyMicros
        case .sell(let holding): holding.valueMicros
        }
    }

    public var spendableCashMicros: Int64? { cashFigures?.spendableCashMicros }

    private var cashFigures: ProposePreview? { preview?.spendableCashMicros != nil ? preview : cashPreview }

    public static let cashShort = "The cabal doesn't have that much cash"

    public static let findingPrice = "Finding the best price…"

    public var helperText: String? {
        if isLoading { return Self.findingPrice }
        return switch trade {
        case .buy: spendableCashMicros.map { "The cabal can spend \(UsdAmountFormatter.format(micros: $0))" }
        case .sell(let holding): holding.helperText
        }
    }

    public var overLimitHelper: String {
        guard case .sell(let holding) = trade else { return Self.cashShort }
        return holding.valueMicros > 0 ? "More than the cabal holds" : ProposeHolding.noPriceHelper
    }

    public var sellQuantityNote: String? {
        guard case .sell(let holding) = trade, amountMicros > 0, holding.valueMicros > 0 else { return nil }
        return "About \(holding.quantity(of: holding.tokenAmount(forMicros: amountMicros)))"
    }

    public var keypadState: AmountKeypadStateValue {
        guard amountMicros > 0 else { return .empty }
        guard isOverLimit else { return .typed }
        switch trade {
        case .buy:
            return .invalid(limitLine: ProposeFlowCopy.overCash, slot: ProposeFlowCopy.upTo(spendableCashMicros ?? 0))
        case .sell(let holding):
            return .invalid(limitLine: ProposeFlowCopy.overHolding, slot: ProposeFlowCopy.upTo(holding.valueMicros))
        }
    }

    public var keypadLine: String? { buyQuantityNote ?? sellQuantityNote }

    public var reviewTitle: String {
        ProposeFlowCopy.reviewProposal
    }

    public var isOverLimit: Bool {
        switch trade {
        case .buy: maxMicros.map { amountMicros > $0 } ?? false
        case .sell(let holding): amountMicros > holding.valueMicros
        }
    }

    public func load() async {
        guard case .buy(let symbol, _, _) = trade else {
            applyInitialAmount()
            return
        }
        defer { applyInitialAmount() }
        do {
            cashPreview = try await service.preview(
                cabalID: cabalID, draft: .buy(symbol: symbol, usdcMicros: 0, thesis: ""), network: nil)
            cashFailed = false
        } catch is CancellationError {
        } catch {
            cashFailed = true
        }
    }

    private func applyInitialAmount() {
        guard let micros = initialAmountMicros, micros > 0 else { return }
        initialAmountMicros = nil
        if let maxMicros, micros > maxMicros {
            amountMicros = micros
            return
        }
        setAmount(micros: micros)
    }

    public func prefetchBestChain() async {
        guard case .buy(let symbol, _, _) = trade, let bestChains else { return }
        await bestChains.refreshIfNeeded(symbol: symbol)
    }

    public var estimate: String? {
        guard isLoading, case .buy(_, let kind, _) = trade, let bestChain, amountMicros > 0,
            let shares = TokenQuantityFormatter.quantity(
                fromAtomics: String(bestChain.shareNanos(forMicros: amountMicros)), decimals: 9)
        else { return nil }
        return "Estimate: about \(TokenQuantityFormatter.label(quantity: shares, kind: kind)) on \(bestChain.chainName)"
    }

    public var buyQuantityNote: String? {
        if let estimate { return estimate }
        guard !isLoading, case .buy(_, let kind, let decimals) = trade, let decimals, let preview,
            previewAmountMicros == amountMicros, amountMicros > 0, let out = preview.quoteOutAmount, out > 0,
            let quantity = TokenQuantityFormatter.quantity(fromAtomics: String(out), decimals: decimals)
        else { return nil }
        let label = "About \(TokenQuantityFormatter.label(quantity: quantity, kind: kind))"
        return previewBestChain.map { "\(label) on \($0.chainName)" } ?? label
    }

    public func setThesis(_ text: String) {
        thesis = ProposeReasonRules.limited(text)
        if hasThesis { showsThesisRequired = false }
    }

    public var hasThesis: Bool { !thesis.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }

    public func requestReview(assetName: String) -> Bool {
        guard reviewEnabled(assetName: assetName) else { return false }
        guard hasThesis else {
            showsThesisRequired = true
            return false
        }
        return true
    }

    public func resolveAsset(kind: AssetKind, decimals: Int) {
        guard case .buy(let symbol, _, _) = trade else { return }
        trade = .buy(symbol: symbol, kind: kind, tokenDecimals: decimals)
    }

    public func setAmount(micros: Int64) {
        amountMicros = max(0, micros)
        previewTask?.cancel()
        previewFailed = false
        isLoading = amountMicros > 0
        bestChain = currentBestChain
        guard amountMicros > 0 else { return }
        let draft = self.draft
        let amount = amountMicros
        let pinned = sellNetwork ?? bestChain?.network
        let fallsBack = sellNetwork == nil
        previewTask = Task { [clock, service, cabalID] in
            do {
                try await clock.sleep(for: .milliseconds(400))
                guard !Task.isCancelled else { return }
                let (preview, isPinned) = try await Self.preview(
                    service, cabalID: cabalID, draft: draft, pinned: pinned, fallsBack: fallsBack)
                guard !Task.isCancelled else { return }
                self.preview = preview
                self.previewBestChain = isPinned ? self.bestChain : nil
                self.previewAmountMicros = amount
                self.isLoading = false
            } catch is CancellationError {
            } catch {
                guard !Task.isCancelled else { return }
                self.isLoading = false
                self.previewFailed = true
            }
        }
    }

    public func retryPreview() { setAmount(micros: amountMicros) }

    private var currentBestChain: AssetBestChain? {
        guard case .buy(let symbol, _, _) = trade else { return nil }
        return bestChains?.bestChain(for: symbol)
    }

    private nonisolated static let unpinnedAdvisories: Set<String> = ["no_route", "invalid_network"]

    private nonisolated static func preview(
        _ service: ProposeService, cabalID: String, draft: ProposalDraft, pinned: Components.Schemas.Network?,
        fallsBack: Bool
    ) async throws -> (ProposePreview, isPinned: Bool) {
        guard fallsBack else {
            return (try await service.preview(cabalID: cabalID, draft: draft, network: pinned), true)
        }
        if let pinned {
            let preview = try? await service.preview(cabalID: cabalID, draft: draft, network: pinned)
            try Task.checkCancellation()
            if let preview, !unpinnedAdvisories.contains(preview.advisoryCode ?? "") { return (preview, true) }
        }
        return (try await service.preview(cabalID: cabalID, draft: draft, network: nil), false)
    }

    private var sellNetwork: Components.Schemas.Network? {
        guard case .sell(let holding) = trade else { return nil }
        return holding.network
    }

    public func chooseLocation(_ network: Components.Schemas.Network) {
        guard case .sell(let holding) = trade, holding.network != network else { return }
        trade = .sell(holding.at(network))
        preview = nil
        previewAmountMicros = nil
        setAmount(micros: 0)
    }

    public func useMax() {
        guard let maxMicros else { return }
        setAmount(micros: maxMicros)
    }

    public func reviewEnabled(assetName: String) -> Bool {
        guard let preview, previewAmountMicros == amountMicros, !isOverLimit, draft.amount > 0, maxMicros != 0
        else { return false }
        return preview.reviewEnabled(
            amountMicros: amountMicros, isLoading: isLoading, assetName: assetName, isSell: trade.isSell)
    }

    public func message(assetName: String) -> String? {
        if !trade.isSell, isOverLimit { return overLimitHelper }
        guard !isLoading else { return nil }
        return preview?.message(assetName: assetName, isSell: trade.isSell)
    }

    public var draft: ProposalDraft {
        switch trade {
        case .buy(let symbol, _, _): .buy(symbol: symbol, usdcMicros: amountMicros, thesis: thesis)
        case .sell(let holding):
            .sell(symbol: holding.symbol, tokenAmount: holding.tokenAmount(forMicros: amountMicros), thesis: thesis)
        }
    }
}
