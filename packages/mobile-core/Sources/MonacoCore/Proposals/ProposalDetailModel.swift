import MonacoAPI
import Observation

public struct ProposalVoterLine: Identifiable, Equatable, Sendable {
    public let person: ProposalPerson
    public let text: String

    public var id: String { person.userID }
}

public struct ProposalPage: Equatable, Sendable {
    public let card: ProposalCard
    public let detail: ProposalDetail
    public let voters: [ProposalVoterLine]

    public var reasonTitle: String { ProposalCardCopy.reasonTitle(card.proposal.kind) }
    public var expected: String { card.asset.expected(of: card.proposal) }
    public var steps: ProposalSteps { detail.steps }
    public var tradeID: String? { detail.trade?.swapID }

    init(detail: ProposalDetail, lookups: ProposalLookups) {
        self.detail = detail
        card = lookups.card(detail.proposal, canVote: detail.canVote, swap: detail.trade?.state)
        let cabal = lookups.cabal(detail.proposal.cabalID)
        voters = detail.voters.map { voter in
            let person = cabal?.person(voter.userID) ?? ProposalPerson(unknown: voter.userID)
            return ProposalVoterLine(person: person, text: ProposalCardCopy.voter(person.name, choice: voter.choice))
        }
    }
}

@Observable
@MainActor
public final class ProposalDetailModel {
    public private(set) var state: LoadState<ProposalPage> = .idle
    public private(set) var celebrations = 0
    public let voting: ProposalVoting
    public let proposalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var generation = 0

    public init(proposalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.proposalID = proposalID
        self.repository = repository
        self.hints = hints
        voting = ProposalVoting(repository: repository)
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public var screen: ProposalPage? {
        if case .loaded(let screen) = state { return screen }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        let previous = screen
        if previous == nil { state = .loading }
        do {
            let detail = try await repository.detail(id: proposalID)
            let lookups = try await repository.lookups(for: [detail.proposal])
            guard issued == generation else { return }
            let next = ProposalPage(detail: detail, lookups: lookups)
            if let previous, Self.justBought(from: previous, to: next) { celebrations += 1 }
            state = .loaded(next)
        } catch {
            guard issued == generation else { return }
            let failure = APIError(error)
            if previous != nil {
                voting.show(ToastCopy.message(for: failure), success: false)
            } else {
                state = .failed(failure)
            }
        }
    }

    public func vote(_ choice: BallotChoice) async {
        guard await voting.cast(choice, on: proposalID) else { return }
        await load()
    }

    public func observe() async {
        guard let cabalID = screen?.detail.proposal.cabalID else { return }
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private static func justBought(from previous: ProposalPage, to next: ProposalPage) -> Bool {
        next.card.proposal.kind == .buy && previous.steps.reached < 2 && next.steps.reached == 2 && !next.steps.failed
    }
}
