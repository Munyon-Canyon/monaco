import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public enum CashOutSubmitResult: Equatable, Sendable {
    case started(CashOutJob)
    case refused(String)
}

@Observable
@MainActor
public final class CashOutModel {
    public static let pauseHint = "pause_changed"
    public static let inProgress = "A cash out is already running for this cabal."
    public static let sliceChanged = "Your slice changed. Check the amount and try again."
    public static let invalidRequest = "Couldn't cash out. Try again."

    public private(set) var state: LoadState<CashOutPreview> = .idle
    public private(set) var isSubmitting = false
    public let cabalID: String

    private let api: APIClient
    private let hints: any HintSource
    private let submission = IdempotentSubmission()
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public var preview: CashOutPreview? {
        if case .loaded(let preview) = state { return preview }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if preview == nil { state = .loading }
        do {
            let wire = try await api.read { [cabalID] client in
                try await client.getCashOutPreview(path: .init(id: cabalID)).ok.body.json
            }
            guard issued == generation else { return }
            state = .loaded(CashOutPreview(wire))
        } catch {
            guard issued == generation, !Task.isCancelled, preview == nil else { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: Self.pauseHint)))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func submit(enteredMicros: Int64) async -> CashOutSubmitResult? {
        guard !isSubmitting, let preview, preview.pause == nil else { return nil }
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        guard let sale = CashOutAmountRule.sale(for: verdict, enteredMicros: enteredMicros) else { return nil }
        isSubmitting = true
        defer { isSubmitting = false }
        let request = sale.request
        let cabalID = cabalID
        do {
            let job = try await api.submit(submission, payload: request, operation: "postCashOut") { client, key in
                try await client.postCashOut(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key),
                    body: .json(request)
                ).accepted.body.json
            }
            return .started(CashOutJob(job))
        } catch {
            return .refused(await refusal(APIError(error)))
        }
    }

    private func refusal(_ error: APIError) async -> String {
        switch Flow14Outcome(error) {
        case .cabalPaused:
            await load()
            return (preview?.pause ?? CabalPause(cause: .ops)).message
        case .cashOutInProgress:
            return Self.inProgress
        case .potValueChanged, .insufficientShares, .priceUnavailable:
            await load()
            return Self.sliceChanged
        case .invalidInput:
            return Self.invalidRequest
        case .privyUnavailable, .rPCUnavailable, .saleShort, .ok, .interrupted, nil:
            return ToastCopy.message(for: error)
        }
    }
}
