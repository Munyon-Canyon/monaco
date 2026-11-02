import MonacoCore
import Testing

@testable import Monaco

@Suite(.serialized)
@MainActor
struct VoteHoldSheetTests {
    @Test func presentingTheSheetCarriesTheProposalAndThePickWithoutACabal() {
        let navigator = AppNavigator()
        navigator.presentVoteHold(proposalID: "proposal-1", pick: .yes)
        #expect(navigator.voteHold?.proposalID == "proposal-1")
        #expect(navigator.voteHold?.pick == .yes)
    }

    @Test func presentingTheSheetClosesDeposit() {
        let navigator = AppNavigator()
        navigator.presentDeposit()
        navigator.presentVoteHold(proposalID: "proposal-1", pick: nil)
        #expect(navigator.deposit == nil)
        #expect(navigator.voteHold?.pick == nil)
    }

    @Test func dismissingAndResettingClearTheSheet() {
        let navigator = AppNavigator()
        navigator.presentVoteHold(proposalID: "proposal-1", pick: .no)
        navigator.dismissVoteHold()
        #expect(navigator.voteHold == nil)
        navigator.presentVoteHold(proposalID: "proposal-1", pick: .no)
        navigator.reset()
        #expect(navigator.voteHold == nil)
    }

    @Test func theTitleKeepsTheCentsOfAnAmountThatIsNotWhole() {
        func title(_ micros: Int64) -> String {
            VoteHoldSheet.title(kind: "buy", symbol: "AAPLx", usdcMicros: micros)
        }
        #expect(title(12_990_000).hasSuffix("\u{00B7} $12.99"))
        #expect(title(12_000_000).hasSuffix("\u{00B7} $12"))
        #expect(title(12_500_000).hasSuffix("\u{00B7} $12.50"))
    }
}
