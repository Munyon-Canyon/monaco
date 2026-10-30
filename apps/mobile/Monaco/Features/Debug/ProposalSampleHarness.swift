#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoFlows
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
    case closedSidesOn
    case closedSidesPrivate
    case nvdaFilled
    case nvdaPassedMoment
    case metaFilled
    case tslaFailed
    case spaceXOpen
    case voteOrdinary
    case voteSettles
    case votePicked
    case voteHolding
    case nonMember
    case permissionSheet
    case holdYes
    case holdVoted
    case holdClosed
    case holdViewOnly
    case holdCold

    static let launchArgument = "-MonacoProposalSample"

    static func matching(_ arguments: [String]) -> ProposalSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ProposalSampleScenario(rawValue: arguments[flag + 1])
    }

    var holdPick: ProposalBallot?? {
        switch self {
        case .holdYes, .holdVoted, .holdClosed, .holdViewOnly: .some(.yes)
        case .holdCold: .some(nil)
        default: nil
        }
    }

    var route: any AppRoute {
        switch self {
        case .needsVote, .needsVoteEmpty: PendingVotesRoute()
        case .allOpen, .allEmpty, .allLoading, .allError: PendingVotesSampleRoute(showsClosed: false)
        case .allClosed, .closedEmpty, .closedLoading, .closedError: PendingVotesSampleRoute(showsClosed: true)
        case .votesOpen, .votesRecorded, .votesEmpty, .votesLoading, .votesError:
            ProposalSampleRoute(votes: .open)
        case .permissionSheet: PermissionSheetSampleRoute()
        case .votePicked: ProposalSampleRoute(votes: .closed, seed: .picked)
        case .voteHolding: ProposalSampleRoute(votes: .closed, seed: .holding)
        case .voteSettles: ProposalSampleRoute(votes: .closed, seed: .settle)
        case .nvdaPassedMoment: ProposalSampleRoute(votes: .closed)
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
            script.proposal = .votableNVDA()
        case .nvdaPassed:
            script.asset = .nvda
            script.proposal = Components.Schemas.ProposalDetail.filledNVDA()
                .closedResult(yes: 4, no: 1, voters: 6, needed: 4, ballot: .yes, order: 4, sides: false)
        case .closedSidesOn:
            script.asset = .nvda
            script.proposal = Components.Schemas.ProposalDetail.filledNVDA()
                .closedResult(yes: 5, no: 1, voters: 6, needed: 4, ballot: .yes, order: 4, sides: true)
                .withPhotoVoters()
        case .closedSidesPrivate:
            script.asset = .nvda
            script.proposal = .passedNVDA().withHiddenSides()
        case .nvdaFilled:
            script.asset = .nvda
            script.proposal = Components.Schemas.ProposalDetail.filledNVDA()
                .closedResult(yes: 4, no: 1, voters: 6, needed: 4, ballot: .yes, order: 4, sides: false)
        case .nvdaPassedMoment:
            script.asset = .nvda
            script.proposal = Components.Schemas.ProposalDetail.filledNVDA().voted(.yes)
        case .metaFilled:
            script.asset = .nvda
            script.proposal = .filledMETA()
        case .tslaFailed:
            script.asset = .teslaTSLA
            script.proposal = Components.Schemas.ProposalDetail.failedTSLA()
                .closedResult(yes: 2, no: 2, voters: 5, needed: 3, ballot: .no, order: 2, sides: false)
        case .spaceXOpen:
            script.asset = .spaceX
            script.proposal = .preIpoSpaceX().votedYesOfSix()
        case .voteOrdinary:
            script.asset = .nvda
            script.proposal = .votableNVDA()
        case .votePicked, .voteHolding:
            script.asset = .nvda
            script.proposal = .votableNVDA()
        case .voteSettles:
            script.asset = .nvda
            script.proposal = .votableNVDA()
            script.vote = .settles
        case .permissionSheet:
            script.asset = .nvda
            script.proposal = Components.Schemas.ProposalDetail.passedNVDA().voted(.yes)
        case .holdYes, .holdCold:
            script.asset = .nvda
            script.proposal = .votableNVDA()
        case .holdVoted:
            script.asset = .nvda
            script.proposal = .votableNVDA().voted(.no)
        case .holdClosed:
            script.asset = .nvda
            script.proposal = .passedNVDA()
        case .holdViewOnly:
            script.asset = .pilatesAMZN
            script.proposal = .pilatesAMZN()
        case .nonMember:
            script.asset = .pilatesAMZN
            script.proposal = .pilatesAMZN()
            script.membersOnlyReplies = true
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
    var seed: ProposalSampleSeed = .none

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: "proposal-1", sections: [ProposalSampleDetailSlot.self, ProposalCommentsSlot.self])
            .environment(\.presentsProposalVotes, votes == .open)
            .environment(\.proposalSampleSeed, seed)
            .environment(\.voteHoldPreview, seed == .holding ? 0.4 : nil)
    }
}

nonisolated enum ProposalSampleSeed: Hashable, Sendable {
    case none, picked, holding, settle
}

private struct ProposalSampleSeedKey: EnvironmentKey {
    static let defaultValue = ProposalSampleSeed.none
}

extension EnvironmentValues {
    fileprivate var proposalSampleSeed: ProposalSampleSeed {
        get { self[ProposalSampleSeedKey.self] }
        set { self[ProposalSampleSeedKey.self] = newValue }
    }
}

@MainActor
private final class ProposalSampleModelBox {
    var model: ProposalDetailModel?
}

private struct ProposalSampleDetail: View {
    let proposalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.proposalSampleSeed) private var seed
    @State private var box = ProposalSampleModelBox()

    var body: some View {
        if seed == .none {
            ProposalDetailSlotView(proposalID: proposalID)
        } else {
            let model = seededModel()
            ProposalDetailSlotView(proposalID: proposalID, model: model)
                .task {
                    guard seed == .settle else { return }
                    await model.load()
                    _ = await model.vote(ProposalBallot.yes.rawValue)
                }
        }
    }

    private func seededModel() -> ProposalDetailModel {
        if let model = box.model { return model }
        let model = ProposalDetailModel(
            id: proposalID, cabalID: "", repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        if seed == .picked || seed == .holding { model.voteSelection = .picked(.yes) }
        box.model = model
        return model
    }
}

nonisolated struct PermissionSheetSampleRoute: AppRoute {
    @MainActor func destination() -> some View {
        PermissionSheetSample()
    }
}

private struct PermissionSheetSample: View {
    @State private var prompt = PushPrePrompt(
        authorization: UndeterminedAuthorization(),
        defaults: UserDefaults(suiteName: "PermissionSheetSample") ?? .standard,
        register: {})

    var body: some View {
        ProposalSampleRoute(votes: .closed).destination()
            .task {
                await prompt.noteFirstVote(
                    sample: PushSample(title: "NVDA passed 5–1", body: "Bought 1.82 sh at $219.40."))
            }
            .sheet(isPresented: $prompt.isPresented) { PushPrePromptSheet(prompt: prompt) }
    }
}

private struct UndeterminedAuthorization: NotificationAuthorizing {
    func status() async -> PushAuthorization { .notDetermined }
    func request() async -> Bool { false }
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
        ProposalSampleDetail(proposalID: context.proposalID)
    }
}

extension Components.Schemas.ProposalDetail {
    fileprivate func voted(_ side: Components.Schemas.BallotChoice) -> Self {
        var detail = self
        detail.myBallot = MyBallotPayload(rawValue: side.rawValue)
        return detail
    }

    fileprivate func closedResult(
        yes: Int, no: Int, voters count: Int, needed: Int, ballot: Components.Schemas.BallotChoice, order: Int,
        sides: Bool
    ) -> Self {
        var detail = voted(ballot)
        detail.tally = .init(yes: yes, no: no, voters: count, needed: needed)
        detail.sidesVisible = sides
        detail.myVote = .init(
            order: order, of: count, yesOrder: ballot == .yes ? order : 0, noOrder: ballot == .no ? order : 0,
            decided: false)
        return detail
    }

    fileprivate func votedYesOfSix() -> Self {
        var detail = voted(.yes)
        detail.canVote = true
        detail.tally = .init(yes: 2, no: 0, voters: 6, needed: 4)
        return detail
    }

    fileprivate func withPhotoVoters() -> Self {
        var detail = self
        let tints = MonacoTheme.CabalTint.allCases
        detail.voters = voters.enumerated().map { index, voter in
            var photo = voter
            photo.photoUrl = SampleImage.flat(tints[index % tints.count], name: "voter-\(index)")?.absoluteString
            return photo
        }
        return detail
    }

    fileprivate func withHiddenSides() -> Self {
        var detail = self
        detail.sidesVisible = false
        detail.voters = voters.enumerated().map { index, voter in
            guard index != 0 else { return voter }
            return .init(
                userId: voter.userId, voted: voter.voted, choice: nil, castAt: nil, displayName: voter.displayName,
                photoUrl: voter.photoUrl)
        }
        return detail
    }

    fileprivate func withoutBallots() -> Self {
        var detail = self
        detail.voters = voters.map {
            .init(
                userId: $0.userId, voted: false, choice: nil, castAt: nil, displayName: $0.displayName,
                photoUrl: $0.photoUrl)
        }
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

enum ProposalVoteFlowScenario: Equatable {
    enum Vote: String, CaseIterable {
        case unauthorized, proposalNotFound, notAVoter, proposalClosed, interrupted
    }

    enum Undo: String, CaseIterable {
        case unauthorized, proposalNotFound, notAVoter, proposalClosed, voteNotFound, undoExpired
    }

    case vote(Vote)
    case undo(Undo)

    static func matching(_ arguments: [String]) -> ProposalVoteFlowScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoFlow"), arguments.indices.contains(flag + 2) else {
            return nil
        }
        let outcome = arguments[flag + 2]
        switch arguments[flag + 1] {
        case "10": return Vote(rawValue: outcome).map(ProposalVoteFlowScenario.vote)
        case "10a": return Undo(rawValue: outcome).map(ProposalVoteFlowScenario.undo)
        default: return nil
        }
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        script.asset = .nvda
        script.proposal = .votableNVDA()
        switch self {
        case .vote(let outcome):
            script.vote = outcome.answer
        case .undo(let outcome):
            script.undo = outcome.answer
        }
        return script
    }
}

extension ProposalVoteFlowScenario.Vote {
    fileprivate var answer: SampleAPIScript.VoteAnswer {
        switch self {
        case .unauthorized:
            .refused(status: 401, code: "unauthorized", message: "Please sign in again.")
        case .proposalNotFound:
            .refused(status: 404, code: "proposal_not_found", message: "This proposal is gone.")
        case .notAVoter:
            .refused(status: 403, code: "not_a_voter", message: "Only voters from when it opened can vote.")
        case .proposalClosed:
            .refused(status: 409, code: "proposal_closed", message: "Voting on this proposal has closed.")
        case .interrupted:
            .dropped
        }
    }
}

extension ProposalVoteFlowScenario.Undo {
    fileprivate var answer: SampleAPIScript.UndoAnswer {
        switch self {
        case .unauthorized:
            .refused(status: 401, code: "unauthorized", message: "Please sign in again.")
        case .proposalNotFound:
            .refused(status: 404, code: "proposal_not_found", message: "This proposal is gone.")
        case .notAVoter:
            .refused(status: 403, code: "not_a_voter", message: "Only voters from when it opened can vote.")
        case .proposalClosed:
            .refused(status: 409, code: "proposal_closed", message: "Voting on this proposal has closed.")
        case .voteNotFound:
            .refused(status: 404, code: "vote_not_found", message: "You have no vote to undo.")
        case .undoExpired:
            .refused(status: 409, code: "undo_expired", message: "It's too late to undo this vote.")
        }
    }
}

extension Components.Schemas.AssetDetail {
    fileprivate static var teslaTSLA: Self {
        var asset = nvda
        asset.symbol = "TSLAx"
        asset.displayName = "Tesla"
        return asset
    }

    fileprivate static var pilatesAMZN: Self {
        var asset = nvda
        asset.symbol = "AMZNx"
        asset.displayName = "Amazon"
        return asset
    }
}

extension Components.Schemas.ProposalDetail {
    fileprivate static func votableNVDA() -> Self {
        var detail = openNVDA()
        detail.canVote = true
        detail.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        return detail
    }

    fileprivate static func pilatesAMZN() -> Self {
        var detail = openNVDA()
        detail.cabalName = "Pilates"
        detail.symbol = "AMZNx"
        detail.usdcMicros = 250_000_000
        detail.stats.priceMicros = 184_500_000
        detail.stats.sharesAtomics = 136_000_000
        detail.stats.closesAt = .now.addingTimeInterval(82_800)
        detail.expiresAt = detail.stats.closesAt
        detail.tally = .init(yes: 3, no: 0, voters: 7, needed: 4)
        detail.replyCount = 2
        detail.canVote = false
        return detail
    }
}

final class ProposalVoteFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard Flow10Scenario.matching(arguments) != nil || Flow10aScenario.matching(arguments) != nil,
            let scenario = ProposalVoteFlowScenario.matching(arguments)
        else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(
            SampleAppFrame(auth: auth, tab: .cabals, routes: [ProposalSampleRoute(votes: .closed)]))
    }
}

final class ProposalSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProposalSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        if let pick = scenario.holdPick {
            return AnyView(
                SampleAppFrame(
                    auth: auth, tab: .home,
                    sheet: {
                        AnyView(
                            VoteHoldSheet(request: VoteHoldRequest(proposalID: "proposal-1", pick: pick))
                                .analyticsScreen("vote_hold"))
                    }))
        }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: .cabals, routes: [scenario.route], opensWithViewer: scenario == .nvdaPassedMoment))
    }
}
#endif
