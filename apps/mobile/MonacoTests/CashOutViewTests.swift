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
        #expect(CashOutView.Copy.title == "Cash out")
        #expect(CashOutView.Copy.sliceComing == "Your slice shows up here soon.")
        #expect(
            CashOutView.Copy.explainer
                == "We sell this much of your slice and move the cash to your account balance. You stay in the cabal.")
        #expect(CashOutView.Copy.submit == "Cash out")
        #expect(CashOutView.Copy.submitComing == "Cash outs open soon.")
    }
}
