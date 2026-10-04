import MonacoAPI
import Observation

public struct PendingVotesSection: Equatable, Sendable {
    public let cards: [ProposalCard]
    public let count: Int
}

@Observable
@MainActor
public final class PendingVotesModel {
    public static let homeLimit = 3

    public private(set) var state: LoadState<PendingVotesSection> = .idle
    public let voting: ProposalVoting
    private let repository: ProposalsRepository
    private let limit: Int?
    private var generation = 0

    public init(repository: ProposalsRepository, limit: Int?) {
        self.repository = repository
        self.limit = limit
        voting = ProposalVoting(repository: repository)
    }

    public func load() async {
        generation += 1
        let issued = generation
        let hadContent: Bool
        if case .loaded = state { hadContent = true } else { hadContent = false }
        if !hadContent { state = .loading }
        do {
            let pending = try await repository.pendingVotes()
            let shown = limit.map { Array(pending.prefix($0)) } ?? pending
            let details = try await details(shown.map(\.id))
            let lookups = try await repository.lookups(for: details.map(\.proposal))
            guard issued == generation else { return }
            let cards = details.map { lookups.card($0.proposal, canVote: $0.canVote, swap: $0.trade?.state) }
            state = .loaded(PendingVotesSection(cards: cards, count: pending.count))
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

    private func details(_ ids: [String]) async throws -> [ProposalDetail] {
        let repository = repository
        let loaded = try await withThrowingTaskGroup(of: (Int, ProposalDetail).self) { group in
            for (index, id) in ids.enumerated() {
                group.addTask { (index, try await repository.detail(id: id)) }
            }
            var byIndex: [Int: ProposalDetail] = [:]
            for try await (index, detail) in group { byIndex[index] = detail }
            return byIndex
        }
        return ids.indices.compactMap { loaded[$0] }
    }
}
