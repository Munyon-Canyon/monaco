import Foundation

/// How closely the cabal screen watches itself while it keeps itself fresh.
///
/// Pure values with no clock and no networking, so `GroupDetailRefreshPolicyTests` can drive every
/// case on the host instead of against a live screen.

// MARK: - Cadence

/// How often the cabal screen re-reads itself.
public enum GroupDetailCadence {
    /// How long the screen keeps watching closely after the last open vote disappears.
    ///
    /// A proposal leaves the open list the moment it passes, and the swap it triggered only
    /// reaches the pot and the activity feed a few seconds later. Dropping straight back to the
    /// resting cadence at that exact moment is what makes a winning vote look like it did nothing.
    public static let voteSettlingWindow: Duration = .seconds(30)

    /// Everything the cabal screen currently has in play.
    public struct Inputs: Equatable, Sendable {
        /// A proposal is collecting votes right now.
        public var hasOpenVotes: Bool
        /// A buy or sell is on its way through.
        public var hasPendingSwap: Bool
        /// Money is on its way into the pot.
        public var hasPendingDeposit: Bool
        /// The last open vote closed within `voteSettlingWindow`, so its outcome is still landing.
        public var isWatchingVoteOutcome: Bool

        public init(
            hasOpenVotes: Bool = false,
            hasPendingSwap: Bool = false,
            hasPendingDeposit: Bool = false,
            isWatchingVoteOutcome: Bool = false
        ) {
            self.hasOpenVotes = hasOpenVotes
            self.hasPendingSwap = hasPendingSwap
            self.hasPendingDeposit = hasPendingDeposit
            self.isWatchingVoteOutcome = isWatchingVoteOutcome
        }
    }

    public static func interval(for inputs: Inputs) -> Duration {
        if inputs.hasPendingDeposit { return DepositPolling.sweepStatusInterval }
        if inputs.hasOpenVotes || inputs.hasPendingSwap || inputs.isWatchingVoteOutcome {
            return LiveRefreshCadence.inPlay
        }
        return LiveRefreshCadence.resting
    }
}
