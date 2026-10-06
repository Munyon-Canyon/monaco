import Foundation
import Observation

@Observable @MainActor
public final class ProposeAmountModel {
    public private(set) var amountMicros: Int64 = 0
    public private(set) var preview: ProposePreview?
    public private(set) var isLoading = false
    public private(set) var thesis = ""
    public let trade: ProposeTrade

    private let service: ProposeService
    private let cabalID: String
    private let clock: any Clock<Duration>
    private var previewTask: Task<Void, Never>?

    public init(service: ProposeService, cabalID: String, trade: ProposeTrade, clock: any Clock<Duration>) {
        self.service = service
        self.cabalID = cabalID
        self.trade = trade
        self.clock = clock
    }

    public var maxMicros: Int64? {
        switch trade {
        case .buy: preview?.maxMicros
        case .sell(let holding): holding.valueMicros
        }
    }

    public var helperText: String? {
        switch trade {
        case .buy: preview?.potHelperText
        case .sell(let holding): holding.helperText
        }
    }

    public var isOverLimit: Bool {
        guard case .sell(let holding) = trade else { return false }
        return amountMicros > holding.valueMicros
    }

    public func setThesis(_ text: String) { thesis = ProposeReasonRules.limited(text) }

    public func setAmount(micros: Int64) {
        amountMicros = max(0, micros)
        previewTask?.cancel()
        preview = nil
        isLoading = amountMicros > 0
        guard amountMicros > 0 else { return }
        let draft = self.draft
        previewTask = Task { [clock, service, cabalID] in
            do {
                try await clock.sleep(for: .milliseconds(400))
                guard !Task.isCancelled else { return }
                let preview = try await service.preview(cabalID: cabalID, draft: draft)
                guard !Task.isCancelled else { return }
                self.preview = preview
                self.isLoading = false
            } catch is CancellationError {
            } catch {
                guard !Task.isCancelled else { return }
                self.isLoading = false
            }
        }
    }

    public func useMax() {
        guard let maxMicros else { return }
        setAmount(micros: maxMicros)
    }

    public func reviewEnabled(assetName: String) -> Bool {
        guard let preview, !isOverLimit, draft.amount > 0 else { return false }
        return preview.reviewEnabled(
            amountMicros: amountMicros, isLoading: isLoading, assetName: assetName, isSell: trade.isSell)
    }

    public func message(assetName: String) -> String? {
        preview?.message(assetName: assetName, isSell: trade.isSell)
    }

    public var draft: ProposalDraft {
        switch trade {
        case .buy(let symbol, _, _): .buy(symbol: symbol, usdcMicros: amountMicros, thesis: thesis)
        case .sell(let holding):
            .sell(symbol: holding.symbol, tokenAmount: holding.tokenAmount(forMicros: amountMicros), thesis: thesis)
        }
    }
}
