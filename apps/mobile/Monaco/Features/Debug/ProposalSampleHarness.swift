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
        default: ProposalRoute(proposalID: "proposal-1")
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
        }
        return script
    }
}

extension Components.Schemas.ProposalDetail.SwapPayload {
    fileprivate static func sample(_ status: StatusPayload) -> Self {
        .init(
            swapId: "swap-1", status: status, failureCode: nil, failureMessage: nil,
            txSignature: status == .confirmed ? "sample-signature" : nil, retryable: false)
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
