import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct CashOutViewTests {
    @Test func theRouteOpensTheCashOutScreen() {
        let view: Any = CashOutRoute(cabalID: "c").destination()
        #expect((view as? CashOutView)?.cabalID == "c")
    }

    @Test func theCopyMatchesTheScreenMap() {
        let slice: Int64 = 200_150_000
        let verdict = CashOutAmountRule.verdict(enteredMicros: 50_030_000, sliceMicros: slice, minimumMicros: 100_000)
        #expect(CashOutAmountRule.helper(for: verdict, sliceMicros: slice) == "Your slice is worth $200.15")
        #expect(
            CashOutAmountRule.explainer(for: verdict)
                == "We sell this much of your slice and move the cash to your account balance. You stay in the cabal.")
        #expect(
            CashOutAmountRule.submitTitle(for: verdict, enteredMicros: 50_030_000, sliceMicros: slice)
                == "Cash out $50.03")
    }

    @Test func theUnderMinimumMessageFloorsTheSlice() {
        let preview = CashOutPreview(sliceMicros: 95_000, shareUnits: 95_000, minMicros: 100_000, pause: nil)
        let message = CashOutContent.belowMinimumMessage(preview)
        #expect(message.contains("worth $0.09"))
        #expect(message.contains("starts at $0.10"))
    }
}
