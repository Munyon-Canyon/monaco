import Foundation
import MonacoAPI

public struct CabalPause: Equatable, Sendable {
    public enum Cause: Equatable, Sendable {
        case externalDeposit
        case ops
    }

    public let cause: Cause

    public init(cause: Cause) {
        self.cause = cause
    }

    init(reasons: [String]) {
        self.init(cause: reasons.contains("external_deposit") ? .externalDeposit : .ops)
    }

    public var message: String {
        switch cause {
        case .externalDeposit:
            "Trading is paused. Someone sent money straight to this cabal's treasury, and it's being returned. "
                + "Funding and cash outs resume after. Passed votes won't trade until trading resumes."
        case .ops:
            "Trading is paused by Monaco. Funding and cash outs resume when it's lifted. "
                + "Passed votes won't trade until trading resumes."
        }
    }
}

public struct CashOutPreview: Equatable, Sendable {
    public let sliceMicros: Int64
    public let shareUnits: Int64
    public let minMicros: Int64
    public let pause: CabalPause?

    public init(sliceMicros: Int64, shareUnits: Int64, minMicros: Int64, pause: CabalPause?) {
        self.sliceMicros = sliceMicros
        self.shareUnits = shareUnits
        self.minMicros = minMicros
        self.pause = pause
    }

    init(_ wire: Components.Schemas.CashOutPreview) {
        self.init(
            sliceMicros: Int64(wire.sliceMicros) ?? 0,
            shareUnits: Int64(wire.shareUnits) ?? 0,
            minMicros: Int64(wire.minMicros) ?? 0,
            pause: wire.pause.map { CabalPause(reasons: $0.reasons) }
        )
    }

    public var hasStake: Bool { sliceMicros > 0 }

    public var sliceIsBelowMinimum: Bool {
        CashOutAmountRule.sliceIsBelowMinimum(sliceMicros: sliceMicros, minimumMicros: minMicros)
    }

    public func verdict(enteredMicros: Int64) -> CashOutAmountRule.Verdict {
        CashOutAmountRule.verdict(enteredMicros: enteredMicros, sliceMicros: sliceMicros, minimumMicros: minMicros)
    }
}

public struct CashOutJob: Equatable, Sendable {
    public enum Status: String, Equatable, Sendable {
        case started, selling, paying, completed, partial, failed
    }

    public let id: String
    public let cabalID: String
    public let status: Status
    public let payoutMicros: Int64
    public let resultCode: String?

    public init(id: String, cabalID: String, status: Status, payoutMicros: Int64, resultCode: String?) {
        self.id = id
        self.cabalID = cabalID
        self.status = status
        self.payoutMicros = payoutMicros
        self.resultCode = resultCode
    }

    init(_ wire: Components.Schemas.CashOutJob) {
        self.init(
            id: wire.id,
            cabalID: wire.cabalId,
            status: Status(rawValue: wire.status.rawValue) ?? .started,
            payoutMicros: Int64(wire.payoutMicros) ?? 0,
            resultCode: wire.resultCode
        )
    }

    public var isRunning: Bool { progress != nil }

    public var progress: String? {
        switch status {
        case .started, .paying: "Cashing out…"
        case .selling: "Selling your slice…"
        case .completed, .partial, .failed: nil
        }
    }

    public var startedToast: String {
        "Cashing out \(amount). It lands in your balance in about a minute"
    }

    public var outcome: CashOutNotice? {
        switch status {
        case .started, .selling, .paying:
            nil
        case .completed:
            CashOutNotice(jobID: id, message: "Cashed out \(amount). It's in your balance.", isSuccess: true)
        case .partial:
            CashOutNotice(
                jobID: id,
                message: "Cashed out \(amount), what the sale raised. You keep the shares it didn't cover.",
                isSuccess: true
            )
        case .failed:
            CashOutNotice(jobID: id, message: Self.failure(resultCode), isSuccess: false)
        }
    }

    private var amount: String { UsdAmountFormatter.format(micros: payoutMicros) }

    static func failure(_ code: String?) -> String {
        switch code {
        case "sale_short": "Your cash out didn't go through. The sale fell short, so your stake stays in the cabal."
        default: "Your cash out didn't go through."
        }
    }
}

public struct CashOutNotice: Equatable, Sendable {
    public let jobID: String
    public let message: String
    public let isSuccess: Bool

    public init(jobID: String, message: String, isSuccess: Bool) {
        self.jobID = jobID
        self.message = message
        self.isSuccess = isSuccess
    }
}
