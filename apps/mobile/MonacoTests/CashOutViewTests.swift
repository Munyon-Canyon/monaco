import MonacoCore
import SwiftUI
import Testing
import UIKit

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
        #expect(
            CashOutAmountRule.helper(
                for: verdict, enteredMicros: 50_030_000, sliceMicros: slice, minimumMicros: 100_000)
                == "Your slice is worth $200.15")
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

    @Test func theReviewPointsAtNoQuoteItDoesNotShow() {
        let copy = (CashOutContent.finalProceedsValue + CashOutContent.reviewNotice).lowercased()
        #expect(!copy.contains("quote"))
        #expect(!copy.contains("estimate"))
        #expect(copy.contains("can change what you receive"))
        #expect(copy.contains("known when"))
        #expect(copy.contains("stay in the cabal"))
    }

    @Test func reopeningCashOutShowsTheRunningJobAndAStartedJobWins() {
        let running = CashOutJob(id: "r", cabalID: "c", status: .selling, payoutMicros: 5_000_000, resultCode: nil)
        let started = CashOutJob(id: "s", cabalID: "c", status: .started, payoutMicros: 7_000_000, resultCode: nil)
        #expect(CashOutContent.shownJob(started: nil, running: running)?.id == "r")
        #expect(CashOutContent.shownJob(started: started, running: running)?.id == "s")
        #expect(CashOutContent.shownJob(started: nil, running: nil) == nil)
    }

    @Test(.timeLimit(.minutes(1))) func reopeningDuringARunningCashOutDrawsItsProgressNotAFreshForm() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let running = CashOutJob(id: "r", cabalID: "c", status: .selling, payoutMicros: 5_000_000, resultCode: nil)
        let window = try ProposalTestWindow.hosting(RunningCashOutHost(running: running))
        defer { window.isHidden = true }
        await ProposalTestWindow.until { AccessibilityTree.labels(in: window).contains("Cash-out requested") }
        for _ in 0..<100 { await Task.yield() }
        let labels = AccessibilityTree.labels(in: window)
        let ids = Set(AccessibilityTree.identifiers(in: window))
        #expect(labels.contains("Cash-out requested"), "\(labels)")
        #expect(!ids.contains("cash-out-loading") && !ids.contains("cash-out-amount"), "\(ids)")
    }

    @Test func aPausedCabalOffersNoAmountEntryOrSubmitBar() {
        let pause = CabalPause(cause: .ops)
        let open = CashOutPreview(sliceMicros: 10_500_000, shareUnits: 1, minMicros: 1_000_000, pause: nil)
        let paused = CashOutPreview(sliceMicros: 10_500_000, shareUnits: 1, minMicros: 1_000_000, pause: pause)
        #expect(CashOutContent.canEnterAmount(open))
        #expect(!CashOutContent.canEnterAmount(paused))
    }
}

private struct RunningCashOutHost: View {
    let running: CashOutJob
    @State private var amountText = ""

    var body: some View {
        CashOutContent(
            model: nil, cabalName: "Big Ballers", runningJob: running, amountText: $amountText, onSubmit: {},
            onFund: {})
    }
}
