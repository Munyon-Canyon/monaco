import MonacoAPI
import Observation

@Observable
@MainActor
public final class ClosedProposalsModel {
    public enum Phase: Equatable, Sendable { case loading, loaded, failed }

    public let pager: CursorPager<ProposalSummary>
    public private(set) var assets: [String: ProposalAsset] = [:]
    public private(set) var members: [String: [ProposalMember]] = [:]
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var started = false

    public init(repository: ProposalsRepository, hints: any HintSource) {
        self.repository = repository
        self.hints = hints
        pager = CursorPager { try await repository.closedProposals(cursor: $0) }
        let hook = ClosedReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.refreshIfStarted() }
    }

    public var phase: Phase {
        if !pager.items.isEmpty { return .loaded }
        switch pager.phase {
        case .idle, .loadingFirst, .loadingMore: return .loading
        case .exhausted: return .loaded
        case .failed: return .failed
        }
    }

    public var canLoadMore: Bool { pager.phase == .idle && !pager.items.isEmpty }

    public func load() async {
        started = true
        await pager.loadFirst()
        await loadContext()
    }

    public func refresh() async {
        guard started, !pager.items.isEmpty else { return await load() }
        await pager.refreshFirstPage()
        await loadContext()
    }

    public func refreshIfStarted() async {
        if started { await refresh() }
    }

    public func loadMore() async {
        await pager.loadMore()
        await loadContext()
    }

    public func proposerName(of summary: ProposalSummary) -> String {
        members[summary.cabalID]?.first { $0.id == summary.proposerID }?.name ?? "Member"
    }

    public func observe() async {
        let streams = ["proposal_updated", "swap_updated"].map { hints.hints(matching: .anyCabal(what: $0)) }
        await refresher.observe(streams)
    }

    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }

    private func loadContext() async {
        let context = await repository.cardContext(
            symbols: Set(pager.items.map(\.symbol)).subtracting(assets.keys),
            cabalIDs: Set(pager.items.map(\.cabalID)).subtracting(members.keys))
        assets.merge(context.assets) { _, new in new }
        members.merge(context.members) { _, new in new }
    }
}

@MainActor
private final class ClosedReloadHook {
    var run: (@MainActor () async -> Void)?
}
