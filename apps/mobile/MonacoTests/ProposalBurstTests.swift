import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProposalBurstTests {
    @Test(arguments: [ProposalStatus.open, .passed, .failed, .executionBlocked])
    func aBuyBurstsWhenItReachesExecutedFromAnotherStatus(from old: ProposalStatus) {
        #expect(ProposalDetailSlotView.boughtJustNow(kind: "buy", from: old, to: .executed))
    }

    @Test func nothingBurstsUnlessABuyJustReachedExecuted() {
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: "sell", from: .passed, to: .executed))
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: nil, from: .passed, to: .executed))
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: "buy", from: .executed, to: .executed))
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: "buy", from: nil, to: .executed))
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: "buy", from: .open, to: .passed))
        #expect(!ProposalDetailSlotView.boughtJustNow(kind: "buy", from: .open, to: nil))
    }
}
