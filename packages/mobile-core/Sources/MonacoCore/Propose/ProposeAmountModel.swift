import Foundation
import Observation

@Observable @MainActor
public final class ProposeAmountModel {
    public private(set) var amountMicros: Int64 = 0
    public private(set) var preview: ProposePreview?
    public private(set) var isLoading = false
    public var thesis = "" { didSet { thesis = ProposeReasonRules.limited(thesis) } }
    public let isSell: Bool

    private let service: ProposeService
    private let cabalID: String
    private let symbol: String
    private let potMicros: Int64
    private let clock: any Clock<Duration>
    private var previewTask: Task<Void, Never>?

    public init(
        service: ProposeService, cabalID: String, symbol: String, potMicros: Int64 = 0, isSell: Bool = false,
        clock: any Clock<Duration>
    ) {
        self.service = service
        self.cabalID = cabalID
        self.symbol = symbol
        self.potMicros = potMicros
        self.isSell = isSell
        self.clock = clock
    }

    public var maxMicros: Int64? { preview?.maxMicros ?? (potMicros > 0 ? potMicros : nil) }
    public var potHelperText: String? {
        maxMicros.map { "The pot has \(UsdAmountFormatter.format(micros: $0))" }
    }

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
        guard let preview else { return false }
        return preview.reviewEnabled(amountMicros: amountMicros, isLoading: isLoading, assetName: assetName)
    }

    public func message(assetName: String) -> String? { preview?.message(assetName: assetName) }

    public var draft: ProposalDraft {
        isSell
            ? .sell(symbol: symbol, tokenAmount: amountMicros, thesis: thesis)
            : .buy(symbol: symbol, usdcMicros: amountMicros, thesis: thesis)
    }
}
