import Foundation
import Testing

@testable import MonacoCore

@Suite("Cabal screen cadence")
struct GroupDetailCadenceTests {
    @Test("A quiet cabal only keeps its balances current")
    func restingByDefault() {
        #expect(GroupDetailCadence.interval(for: .init()) == LiveRefreshCadence.resting)
    }

    @Test("An open vote is watched closely")
    func openVotesAreInPlay() {
        #expect(GroupDetailCadence.interval(for: .init(hasOpenVotes: true)) == LiveRefreshCadence.inPlay)
    }

    @Test("A swap on its way is watched closely")
    func pendingSwapIsInPlay() {
        #expect(GroupDetailCadence.interval(for: .init(hasPendingSwap: true)) == LiveRefreshCadence.inPlay)
    }

    @Test("Money on its way into the pot is watched at the sweep cadence")
    func pendingDepositUsesSweepCadence() {
        let inputs = GroupDetailCadence.Inputs(hasOpenVotes: true, hasPendingDeposit: true)
        #expect(GroupDetailCadence.interval(for: inputs) == DepositPolling.sweepStatusInterval)
    }

    /// The bug: the deciding vote drops the proposal out of the open list, and the screen went
    /// quiet for a full resting interval at the moment the swap it caused was starting.
    @Test("The outcome of a vote that just closed is still watched closely")
    func votesThatJustClosedStayInPlay() {
        let justVoted = GroupDetailCadence.Inputs(
            hasOpenVotes: false,
            hasPendingSwap: false,
            isWatchingVoteOutcome: true
        )
        #expect(GroupDetailCadence.interval(for: justVoted) == LiveRefreshCadence.inPlay)
    }

    @Test("Once the settling window is over the screen goes back to resting")
    func settlingWindowEnds() {
        let settled = GroupDetailCadence.Inputs(isWatchingVoteOutcome: false)
        #expect(GroupDetailCadence.interval(for: settled) == LiveRefreshCadence.resting)
        #expect(GroupDetailCadence.voteSettlingWindow > LiveRefreshCadence.resting)
    }
}
