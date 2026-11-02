import Foundation
import MonacoAPI
import Observation

public enum FundCabalState: Equatable, Sendable {
    case empty
    case typed(sliceNow: Int32, sliceAfter: Int32)
    case invalid(availableMicros: Int64, overMicros: Int64)
    case belowMinimum(minMicros: Int64)
    case paused
    case submitting
    case unconfirmed(String)
    case settled(amountMicros: Int64, sliceNow: Int32?, cashAfterMicros: Int64)
    case failed(String)
}

@Observable
@MainActor
public final class FundCabalModel {
    public static let debounce: Duration = .milliseconds(250)

    public var amountText = ""
    public private(set) var preview: Components.Schemas.FundPreview?
    public private(set) var watchedTransferID: String?
    public private(set) var shareUnavailable = false

    public let cabalID: String
    public let cabalName: String

    private var resting: FundCabalState = .empty
    private var cashAfterMicros: Int64 = 0
    private var isFirstFund = false
    private var previewedMicros: Int64 = 0
    private var presented = false
    private var closed = false
    private var generation = 0
    private var pendingPreview: Task<Void, Never>?

    @ObservationIgnored private let api: APIClient
    @ObservationIgnored private let hints: any HintSource
    @ObservationIgnored private let funding: Funding
    @ObservationIgnored private let settlements: SettlementWatcher
    @ObservationIgnored private let clock: any Clock<Duration>
    @ObservationIgnored private let refresher: HintRefresher

    public init(
        api: APIClient, hints: any HintSource, cabalID: String, cabalName: String, submission: IdempotentSubmission,
        settlements: SettlementWatcher, clock: any Clock<Duration>
    ) {
        self.api = api
        self.hints = hints
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.settlements = settlements
        self.clock = clock
        funding = Funding(cabalID: cabalID, source: FundSource(api: api), hints: hints, submission: submission)
        let hook = FundCabalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public var amountMicros: Int64? { AmountText.micros(amountText) }

    public var maxMicros: Int64 { preview.flatMap { Int64($0.availableMicros) } ?? 0 }

    public var sliceNowBps: Int32? { preview?.sliceBpsNow }

    public var state: FundCabalState {
        guard let id = watchedTransferID else { return resting }
        switch settlements.fund[id] {
        case .settled(let transfer):
            return .settled(
                amountMicros: transfer.amountMicros, sliceNow: transfer.sliceBps, cashAfterMicros: cashAfterMicros)
        case .failed:
            return .failed(FundProgress.didNotGoThrough)
        case .idle, .submitting, .submitted, nil:
            return .submitting
        }
    }

    public var firstFund: Bool {
        if case .settled = state { return isFirstFund }
        return false
    }

    public var canSubmit: Bool {
        guard let micros = amountMicros, micros > 0 else { return false }
        switch state {
        case .unconfirmed: return true
        case .typed, .failed: return previewedMicros == micros
        case .empty, .invalid, .belowMinimum, .paused, .submitting, .settled: return false
        }
    }

    public var line: String {
        switch state {
        case .empty, .invalid:
            if shareUnavailable { return FundCabalCopy.fromCash }
            return FundCabalCopy.shareLine(now: sliceNowBps ?? 0)
        case .typed(let now, let after):
            if shareUnavailable { return FundCabalCopy.fromCash }
            return FundCabalCopy.shareLine(now: now, after: after)
        case .belowMinimum(let minMicros):
            return FundCabalCopy.belowMinimum(minMicros: minMicros)
        case .paused:
            return FundCabalCopy.paused(cabalName: cabalName)
        case .submitting:
            return FundCabalCopy.funding
        case .unconfirmed(let message), .failed(let message):
            return message
        case .settled(_, let now, _):
            return FundCabalCopy.settledLine(now: now)
        }
    }

    public var limitLine: String? {
        guard case .invalid(let available, let over) = state else { return nil }
        return FundCabalCopy.limitLine(availableMicros: available, overMicros: over)
    }

    public var slotHint: String {
        guard case .invalid(let available, _) = state else { return FundCabalCopy.enterAnAmount }
        return FundCabalCopy.slotHint(availableMicros: available)
    }

    public var buttonTitle: String? {
        guard canSubmit, let micros = amountMicros else { return nil }
        if case .unconfirmed = state { return FundCabalCopy.checkAgain }
        return FundCabalCopy.button(micros: micros)
    }

    public var checkLabel: String? {
        guard case .settled(let micros, _, _) = state else { return nil }
        return FundCabalCopy.check(micros: micros, cabalName: cabalName)
    }

    public func load() async {
        await refreshPreview(includingAmount: true)
    }

    public func amountChanged() {
        pendingPreview?.cancel()
        generation += 1
        clearFailedTransfer()
        guard watchedTransferID == nil else { return }
        guard let micros = amountMicros, micros > 0 else {
            pendingPreview = nil
            applyPreview(preview, micros: 0)
            return
        }
        let issued = generation
        pendingPreview = Task { [weak self, clock] in
            do { try await clock.sleep(for: Self.debounce) } catch { return }
            guard let self, issued == self.generation else { return }
            await self.refreshPreview(includingAmount: true)
        }
    }

    public func submit() async {
        guard canSubmit, let micros = amountMicros else { return }
        clearFailedTransfer()
        isFirstFund = FirstFundModel.heldNoShare(sliceBps: preview?.sliceBpsNow)
        cashAfterMicros = max(maxMicros - micros, 0)
        resting = .submitting
        switch await funding.submit(micros: micros) {
        case .accepted(let transfer):
            settlements.watch(
                SettlementTicket(
                    kind: .fund, resourceID: transfer.transferID, cabalID: cabalID, cabalName: cabalName))
            if !closed {
                settlements.present(transfer.transferID)
                presented = true
            }
            watchedTransferID = transfer.transferID
        case .refused(let failure):
            switch failure {
            case .needsMoney(let message), .toast(let message): resting = .failed(message)
            }
        case .unconfirmed(let message):
            if let pending = funding.pendingMicros, pending != micros {
                amountText = AmountText.plain(AmountText.roundDownToCents(AmountText.dollars(micros: pending)))
                resting = .unconfirmed(FundCabalCopy.pendingBlocked(micros: pending))
            } else {
                resting = .unconfirmed(message)
            }
        }
    }

    private func clearFailedTransfer() {
        guard let id = watchedTransferID, case .failed = settlements.fund[id] else { return }
        if presented {
            settlements.dismiss(id)
            presented = false
        }
        watchedTransferID = nil
        funding.reset()
        resting = .empty
        applyPreview(preview, micros: amountMicros ?? 0)
    }

    public func dismiss() {
        closed = true
        guard presented, let id = watchedTransferID else { return }
        presented = false
        settlements.dismiss(id)
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: BalanceSource.refreshingHint)))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func refreshPreview(includingAmount: Bool) async {
        if case .submitting = state { return }
        generation += 1
        let issued = generation
        let settled: Bool
        if case .settled = state { settled = true } else { settled = false }
        let micros = includingAmount && !settled ? (amountMicros ?? 0) : 0
        do {
            let wire = try await api.read { client in
                try await client.getFundPreview(
                    path: .init(id: cabalID), query: .init(amountMicros: micros > 0 ? String(micros) : nil)
                ).ok.body.json
            }
            guard issued == generation else { return }
            preview = wire
            previewedMicros = micros
            shareUnavailable = false
            if settled {
                cashAfterMicros = maxMicros
            } else {
                applyPreview(wire, micros: micros)
            }
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            let failure = APIError(error)
            if Self.isPricesStale(failure) {
                shareUnavailable = true
                previewedMicros = micros
                if preview == nil { resting = .empty }
            } else if preview == nil {
                resting = .failed(BalanceSource.message(for: failure))
            }
        }
    }

    private static func isPricesStale(_ error: APIError) -> Bool {
        if case .problem(let problem) = error { return problem.code == .known(.pricesStale) }
        return false
    }

    private func applyPreview(_ wire: Components.Schemas.FundPreview?, micros: Int64) {
        if case .submitting = resting { return }
        guard let wire else {
            resting = .empty
            return
        }
        let available = Int64(wire.availableMicros) ?? 0
        let over = Int64(wire.overMicros) ?? 0
        let minimum = Int64(wire.minMicros) ?? 0
        if wire.paused {
            resting = .paused
        } else if micros == 0 {
            resting = .empty
        } else if over > 0 {
            resting = .invalid(availableMicros: available, overMicros: over)
        } else if micros < minimum {
            resting = .belowMinimum(minMicros: minimum)
        } else {
            resting = .typed(sliceNow: wire.sliceBpsNow, sliceAfter: wire.sliceBpsAfter ?? wire.sliceBpsNow)
        }
    }
}

@MainActor
private final class FundCabalReloadHook {
    var run: (@MainActor () async -> Void)?
}
