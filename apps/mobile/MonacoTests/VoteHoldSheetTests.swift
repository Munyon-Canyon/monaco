import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

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

    @Test func theCardLineReadsTheCabalTheVotedCountTheRepliesAndTheTimeLeft() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        var proposal = Components.Schemas.Proposal.sample()
        proposal.cabalName = "AC Capital"
        proposal.expiresAt = now.addingTimeInterval(4 * 3_600)
        let summary = ProposalSummary(proposal, canVote: true)
        let tally = ProposalTally(yes: 4, no: 1, voters: 8, needed: 5)
        let line = VoteHoldSheet.line(summary, tally: tally, replies: 6, now: now)
        #expect(line.short == "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 6 replies \u{00B7} 4h")
    }
}
