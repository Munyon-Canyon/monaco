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
}
