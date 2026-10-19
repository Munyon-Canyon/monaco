#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ProposalSampleScenario: String, CaseIterable {
    case open
    case voted
    case passed
    case swapping
    case executed
    case failedRetryable
    case failedFinal
    case expired
    case readOnly
    case sell
    case loading
    case unavailable
    case needsVote
    case needsVoteEmpty
    case notVoter
    case rejected
    case blocked
    case votesOpen
    case votesRecorded
    case votesEmpty
    case votesExpanded
    case votesLoading
    case votesError
    case allOpen
    case allClosed
    case allEmpty
    case allLoading
    case allError

    static let launchArgument = "-MonacoProposalSample"

    static func matching(_ arguments: [String]) -> ProposalSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ProposalSampleScenario(rawValue: arguments[flag + 1])
    }

    var route: any AppRoute {
        switch self {
        case .needsVote, .needsVoteEmpty: PendingVotesRoute()
        case .allOpen, .allEmpty, .allLoading, .allError: PendingVotesSampleRoute(showsClosed: false)
        case .allClosed: PendingVotesSampleRoute(showsClosed: true)
        case .votesOpen, .votesRecorded, .votesEmpty, .votesLoading, .votesError:
            ProposalSampleRoute(votes: .medium)
        case .votesExpanded: ProposalSampleRoute(votes: .large)
        default: ProposalSampleRoute(votes: .closed)
        }
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        switch self {
        case .open: script.proposal = .sample(canVote: true)
        case .voted: script.proposal = .sample(myBallot: .yes)
        case .passed: script.proposal = .sample(status: .passed)
        case .swapping: script.proposal = .sample(status: .passed, swap: .sample(.submitted))
        case .executed: script.proposal = .sample(status: .executed, swap: .sample(.confirmed))
        case .failedRetryable: script.proposal = .failedSwap(retryable: true)
        case .failedFinal: script.proposal = .failedSwap(retryable: false)
        case .expired: script.proposal = .sample(status: .expired)
        case .readOnly: script.proposal = .sample()
        case .sell: script.proposal = .sample(kind: .sell, canVote: true)
        case .loading: script.mode = .hang
        case .unavailable: break
        case .needsVote:
            script.proposal = .sample(canVote: true)
            script.pendingVotes = Components.Schemas.PendingVote.samples
        case .needsVoteEmpty: break
        case .notVoter: script.proposal = .sample()
        case .rejected: script.proposal = .sample(status: .failed)
        case .blocked: script.proposal = .sample(status: .executionBlocked, swap: .sample(.failed))
        case .votesOpen, .votesExpanded: script.proposal = .sample(canVote: true)
        case .votesRecorded: script.proposal = .sample(myBallot: .yes, canVote: true)
        case .votesEmpty: script.proposal = .sample(canVote: true).withoutBallots()
        case .votesLoading: script.mode = .hang
        case .votesError: break
        case .allOpen:
            script.proposal = .sample(canVote: true)
            script.pendingVotes = Components.Schemas.PendingVote.samples
        case .allClosed: script.proposal = .sample(canVote: true)
        case .allEmpty: break
        case .allLoading: script.mode = .hang
        case .allError: script.pendingVotesFail = true
        }
        return script
    }
}

nonisolated struct ProposalSampleRoute: AppRoute {
    enum Votes: Hashable, Sendable {
        case closed
        case medium
        case large
    }

    let votes: Votes

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: "proposal-1", sections: [ProposalSampleDetailSlot.self, ProposalCommentsSlot.self])
            .environment(\.presentedProposalVotes, detent)
    }

    private var detent: PresentationDetent? {
        switch votes {
        case .closed: nil
        case .medium: .medium
        case .large: .large
        }
    }
}

nonisolated struct PendingVotesSampleRoute: AppRoute {
    let showsClosed: Bool

    @MainActor func destination() -> some View {
        PendingVotesScreen(segment: showsClosed ? .closed : .open, closed: showsClosed ? ClosedFixture.rows : [])
    }
}

private enum ClosedFixture {
    static let rows = [
        row("closed-1", "NVDAx", "NVIDIA", "$400.00", .approved),
        row("closed-2", "MSFTx", "Microsoft", "$250.00", .expired),
        row("closed-3", "AAPLx", "Apple", "$150.00", .blocked),
    ]

    private static func row(_ id: String, _ symbol: String, _ name: String, _ amount: String, _ state: ProposalState)
        -> ProposalListRow
    {
        ProposalListRow(
            id: id, symbol: symbol, assetName: name, title: "\(name) · \(amount)",
            subtitle: ProposalCardCopy.rowSubtitle(state, proposer: "Mia", statusMessage: nil),
            badge: .listRow(state))
    }
}

private enum ProposalSampleDetailSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalDetailSlotView(
            proposalID: context.proposalID,
            authorReturn: ProposalAuthorReturn(text: "+8.2% · 1M", tone: .gain),
            executionFacts: ProposalExecutionFacts(price: "$182.40", units: "2.1930"))
    }
}

extension Components.Schemas.ProposalDetail {
    fileprivate func withoutBallots() -> Self {
        var detail = self
        detail.voters = voters.map { .init(userId: $0.userId, choice: nil, castAt: nil) }
        detail.tally = .init(yes: 0, no: 0, voters: tally.voters, needed: tally.needed)
        return detail
    }
}

extension Components.Schemas.ProposalDetail.SwapPayload {
    fileprivate static func sample(_ status: StatusPayload) -> Self {
        .init(
            swapId: "swap-1", status: status, failureCode: status == .failed ? "jupiter_failed" : nil,
            failureMessage: nil, txSignature: status == .confirmed ? "sample-signature" : nil, retryable: false)
    }
}

final class ProposalSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProposalSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [scenario.route]))
    }
}
#endif
