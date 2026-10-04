import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProposalListModel {
    public let pagers: [ProposalFilter: CursorPager<ProposalCard>]
    public let voting: ProposalVoting
    public let cabalID: String
    private let hints: any HintSource
    private let refresher: HintRefresher

    public init(cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.cabalID = cabalID
        self.hints = hints
        voting = ProposalVoting(repository: repository)
        var pagers: [ProposalFilter: CursorPager<ProposalCard>] = [:]
        for filter in ProposalFilter.allCases {
            pagers[filter] = CursorPager { cursor in
                let page = try await repository.list(cabalID: cabalID, filter: filter, cursor: cursor)
                let lookups = try await repository.lookups(for: page.items, cabals: [cabalID])
                return (page.items.map { lookups.card($0) }, page.nextCursor)
            }
        }
        self.pagers = pagers
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.refresh() }
    }

    public func pager(_ filter: ProposalFilter) -> CursorPager<ProposalCard> {
        pagers[filter] ?? CursorPager { _ in ([], nil) }
    }

    public func load(_ filter: ProposalFilter) async {
        await pager(filter).loadFirst()
    }

    public func refresh() async {
        await withTaskGroup(of: Void.self) { group in
            for pager in pagers.values {
                group.addTask { await pager.refreshFirstPage() }
            }
        }
    }

    public func vote(_ choice: BallotChoice, on card: ProposalCard) async {
        guard await voting.cast(choice, on: card.id) else { return }
        await refresh()
    }

    public func observe() async {
        await refresher.observe(
            CabalProposalsModel.refreshingHints.map { hints.hints(matching: .cabal(id: cabalID, what: $0)) })
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
