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
    case votesLoading
    case votesError
    case allOpen
    case allClosed
    case allEmpty
    case allLoading
    case allError
    case closedEmpty
    case closedLoading
    case closedError
    case nvdaOpen
    case nvdaPassed
    case nvdaFilled
    case metaFilled
    case tslaFailed
    case spaceXOpen

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
        case .allClosed, .closedEmpty, .closedLoading, .closedError: PendingVotesSampleRoute(showsClosed: true)
        case .votesOpen, .votesRecorded, .votesEmpty, .votesLoading, .votesError:
            ProposalSampleRoute(votes: .open)
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
        case .votesOpen: script.proposal = .sample(canVote: true)
        case .votesRecorded: script.proposal = .sample(myBallot: .yes, canVote: true)
        case .votesEmpty: script.proposal = .sample(canVote: true).withoutBallots()
        case .votesLoading: script.mode = .hang
        case .votesError: break
        case .allOpen:
            script.proposal = .sample(canVote: true)
            script.pendingVotes = Components.Schemas.PendingVote.samples
        case .allClosed:
            script.proposal = .sample(canVote: true)
            script.closedProposals = ClosedFixture.proposals
            script.assets = ClosedFixture.assets
        case .closedEmpty: break
        case .closedLoading: script.closedProposalsHang = true
        case .closedError: script.closedProposalsFail = true
        case .allEmpty: break
        case .allLoading: script.mode = .hang
        case .allError: script.pendingVotesFail = true
        case .nvdaOpen:
            script.asset = .nvda
            script.proposal = .openNVDA()
        case .nvdaPassed:
            script.asset = .nvda
            script.proposal = .passedNVDA()
        case .nvdaFilled:
            script.asset = .nvda
            script.proposal = .filledNVDA()
        case .metaFilled:
            script.asset = .nvda
            script.proposal = .filledMETA()
        case .tslaFailed:
            script.asset = .nvda
            script.proposal = .failedTSLA()
        case .spaceXOpen:
            script.asset = .spaceX
            script.proposal = .preIpoSpaceX()
        }
        return script
    }
}

nonisolated struct ProposalSampleRoute: AppRoute {
    enum Votes: Hashable, Sendable {
        case closed
        case open
    }

    let votes: Votes

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: "proposal-1", sections: [ProposalSampleDetailSlot.self, ProposalCommentsSlot.self])
            .environment(\.presentsProposalVotes, votes == .open)
    }
}

nonisolated struct PendingVotesSampleRoute: AppRoute {
    let showsClosed: Bool

    @MainActor func destination() -> some View {
        PendingVotesScreen(segment: showsClosed ? .closed : .open)
    }
}

private enum ClosedFixture {
    static let proposals = [
        proposal("closed-1", "NVDAx", 400_000_000, .passed),
        proposal("closed-2", "MSFTx", 250_000_000, .expired),
        proposal("closed-3", "AAPLx", 150_000_000, .executionBlocked),
    ]

    static let assets = [
        "NVDAx": asset("NVDAx", "NVIDIA"), "MSFTx": asset("MSFTx", "Microsoft"), "AAPLx": asset("AAPLx", "Apple"),
    ]

    private static func proposal(
        _ id: String, _ symbol: String, _ usdcMicros: Int64, _ status: Components.Schemas.ProposalStatus
    ) -> Components.Schemas.Proposal {
        var proposal = Components.Schemas.Proposal.sample(status: status)
        proposal.id = id
        proposal.symbol = symbol
        proposal.usdcMicros = usdcMicros
        return proposal
    }

    private static func asset(_ symbol: String, _ name: String) -> Components.Schemas.AssetDetail {
        var detail = Components.Schemas.AssetDetail.googl
        detail.symbol = symbol
        detail.displayName = name
        return detail
    }
}

private enum ProposalSampleDetailSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalDetailSlotView(proposalID: context.proposalID)
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
            failureMessage: nil, txSignature: status == .confirmed ? "sample-signature" : nil, retryable: false,
            usdcMicros: status == .confirmed ? 400_000_000 : nil, tokenAmount: status == .confirmed ? 219_300_000 : nil,
            tokenDecimals: 8, priceMicros: status == .confirmed ? 182_400_000 : nil)
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
