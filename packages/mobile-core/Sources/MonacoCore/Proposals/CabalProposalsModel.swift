import MonacoAPI
import Observation

public enum CabalProposalsSection: Equatable, Sendable {
    case hidden
    case empty
    case cards([ProposalCard], awaitingYou: Int)
}

@Observable
@MainActor
public final class CabalProposalsModel {
    public static let refreshingHints = ["proposal_created", "proposal_updated"]

    public private(set) var state: LoadState<CabalProposalsSection> = .idle
    public let voting: ProposalVoting
    public let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var generation = 0

    public init(cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
        voting = ProposalVoting(repository: repository)
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public func load() async {
        generation += 1
        let issued = generation
        let hadContent: Bool
        if case .loaded = state { hadContent = true } else { hadContent = false }
        if !hadContent { state = .loading }
        do {
            let page = try await repository.list(cabalID: cabalID, filter: .open, cursor: nil)
            let lookups = try await repository.lookups(for: page.items, cabals: [cabalID])
            guard issued == generation else { return }
            state = .loaded(Self.section(page.items.map { lookups.card($0) }, cabal: lookups.cabal(cabalID)))
        } catch {
            guard issued == generation else { return }
            let failure = APIError(error)
            if hadContent {
                voting.show(ToastCopy.message(for: failure), success: false)
            } else {
                state = .failed(failure)
            }
        }
    }

    public func vote(_ choice: BallotChoice, on card: ProposalCard) async {
        guard await voting.cast(choice, on: card.id) else { return }
        await load()
    }

    public func observe() async {
        await refresher.observe(Self.refreshingHints.map { hints.hints(matching: .cabal(id: cabalID, what: $0)) })
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    static func section(_ cards: [ProposalCard], cabal: ProposalCabal?) -> CabalProposalsSection {
        let newestFirst = cards.sorted { $0.proposal.createdAt > $1.proposal.createdAt }
        guard !newestFirst.isEmpty else { return cabal?.isMember == true ? .empty : .hidden }
        return .cards(newestFirst, awaitingYou: newestFirst.filter(\.awaitsViewer).count)
    }
}
