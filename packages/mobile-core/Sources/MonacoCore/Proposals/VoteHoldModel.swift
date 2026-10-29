import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class VoteHoldModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case ready(picked: ProposalBallot?)
        case alreadyVoted(ProposalBallot)
        case closed
        case viewOnly
        case failed
    }

    public let proposalID: String
    public let detail: ProposalDetailModel
    private let pick: ProposalBallot?
    private let now: @Sendable () -> Date

    public init(
        proposalID: String, pick: ProposalBallot?, repository: ProposalsRepository, hints: any HintSource,
        now: @escaping @Sendable () -> Date = Date.init
    ) {
        self.proposalID = proposalID
        self.pick = pick
        self.now = now
        detail = ProposalDetailModel(id: proposalID, repository: repository, hints: hints)
        detail.voteSelection = pick.map { .picked($0) } ?? .none
    }

    public var summary: ProposalSummary? { detail.summary }
    public var tally: ProposalTally? { summary?.tally }
    public var state: ProposalState? { summary?.state(now: now()) }

    public var phase: Phase {
        guard let summary, let state = state else { return detail.errorMessage == nil ? .loading : .failed }
        if detail.lastVote != nil, let side = selectedSide { return .ready(picked: side) }
        switch state {
        case .open:
            return .ready(picked: selectedSide)
        case .voted(let side):
            if case .picked = detail.voteSelection { return .ready(picked: selectedSide) }
            return .alreadyVoted(side)
        case .viewOnly:
            return summary.canVote ? .ready(picked: selectedSide) : .viewOnly
        default:
            return .closed
        }
    }

    private var selectedSide: ProposalBallot? {
        switch detail.voteSelection {
        case .none: nil
        case .picked(let side), .pending(let side), .committed(let side): side
        }
    }

    public func load() async {
        await detail.load()
        guard case .picked = detail.voteSelection, let state, !isOpenAndVotable(state) else { return }
        if case .voted(let side) = state {
            detail.voteSelection = .committed(side)
        } else {
            detail.voteSelection = .none
        }
    }

    private func isOpenAndVotable(_ state: ProposalState) -> Bool {
        if case .open = state { return true }
        return false
    }

    public func commit(_ side: ProposalBallot) async -> Bool { await detail.vote(side.rawValue) }

    public func undo() async -> Bool { await detail.undo() }
}
