import MonacoAPI

public enum CashOutAmountRule {
    public enum Verdict: Equatable, Sendable {
        case noAmount
        case belowMinimum
        case overSlice
        case sellsWholeSlice
        case ok

        public var maySubmit: Bool {
            switch self {
            case .ok, .sellsWholeSlice: true
            case .noAmount, .belowMinimum, .overSlice: false
            }
        }
    }

    public enum Sale: Equatable, Sendable {
        case all
        case usdc(Int64)

        public var request: Components.Schemas.CashOutRequest {
            switch self {
            case .all: .init(all: true)
            case .usdc(let micros): .init(usdcMicros: String(micros))
            }
        }
    }

    public static let tooSmall = "Too small to cash out"

    public static func verdict(enteredMicros: Int64, sliceMicros: Int64, minimumMicros: Int64) -> Verdict {
        guard enteredMicros > 0 else { return .noAmount }
        guard enteredMicros <= sliceMicros else { return .overSlice }
        guard enteredMicros >= minimumMicros else { return .belowMinimum }
        return sliceMicros - enteredMicros < minimumMicros ? .sellsWholeSlice : .ok
    }

    public static func sliceIsBelowMinimum(sliceMicros: Int64, minimumMicros: Int64) -> Bool {
        sliceMicros > 0 && sliceMicros < minimumMicros
    }

    public static func effectiveMicros(for verdict: Verdict, enteredMicros: Int64, sliceMicros: Int64) -> Int64 {
        switch verdict {
        case .sellsWholeSlice: sliceMicros
        case .ok: enteredMicros
        case .noAmount, .belowMinimum, .overSlice: 0
        }
    }

    public static func sale(for verdict: Verdict, enteredMicros: Int64) -> Sale? {
        switch verdict {
        case .sellsWholeSlice: .all
        case .ok: .usdc(enteredMicros)
        case .noAmount, .belowMinimum, .overSlice: nil
        }
    }

    public static func problem(for verdict: Verdict) -> String? {
        verdict == .belowMinimum ? tooSmall : nil
    }

    public static func explainer(for verdict: Verdict) -> String {
        switch verdict {
        case .sellsWholeSlice:
            "This cashes out your whole slice. The cash moves to your account balance, and you stay in the cabal with nothing in the pot."
        case .noAmount, .belowMinimum, .overSlice, .ok:
            "We sell this much of your slice and move the cash to your account balance. You stay in the cabal."
        }
    }

    public static func helper(for verdict: Verdict, sliceMicros: Int64) -> String {
        let slice = UsdAmountFormatter.format(flooredMicros: sliceMicros)
        return verdict == .sellsWholeSlice
            ? "We'll cash out your whole slice, \(slice)"
            : "Your slice is worth \(slice)"
    }

    public static func submitTitle(for verdict: Verdict, enteredMicros: Int64, sliceMicros: Int64) -> String {
        let micros = effectiveMicros(for: verdict, enteredMicros: enteredMicros, sliceMicros: sliceMicros)
        guard micros > 0 else { return "Cash out" }
        return "Cash out \(UsdAmountFormatter.format(flooredMicros: micros))"
    }
}
