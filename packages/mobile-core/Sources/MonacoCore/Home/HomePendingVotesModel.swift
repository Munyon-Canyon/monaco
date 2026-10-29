import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class HomePendingVotesModel {
    public enum Phase: Equatable, Sendable { case loading, loaded, failed }

    public private(set) var phase: Phase = .loading
    public private(set) var cards: [HomeVoteCardModel] = []
    private let api: APIClient
    private let hints: any HintSource
    private let now: @Sendable () -> Date
    private let releaseAfter: TimeInterval
    private let refresher: HintRefresher
    public var cardSetup: (@MainActor (HomeVoteCardModel) -> Void)?
    private var hasLeftHome = false

    public init(
        api: APIClient, hints: any HintSource, now: @escaping @Sendable () -> Date = Date.init,
        releaseAfter: TimeInterval = HomeVoteCardModel.defaultReleaseAfter
    ) {
        self.api = api
        self.hints = hints
        self.now = now
        self.releaseAfter = releaseAfter
        let hook = HomePendingVotesReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public var shown: HomeVoteCardModel? { cards.first }
    public var waiting: [HomeVoteCardModel] { cards.filter { !$0.isCommitted } }
    public var waitingCount: Int { waiting.count }
    public var rows: [HomePendingVote] { waiting.map(\.row) }
    public var votedIDs: Set<String> { Set(cards.filter(\.isCommitted).map(\.id)) }

    public func load() async {
        do {
            let pending = try await api.read { client in
                try await client.getMyPendingVotes().ok.body.json
            }
            let rows = pending.map(HomePendingVote.init).sorted { $0.expiresAt < $1.expiresAt }
            apply(rows)
            phase = .loaded
        } catch {
            if phase != .loaded { phase = .failed }
        }
    }

    public func observe() async {
        let streams = ["proposal_created", "proposal_updated", "swap_updated"]
            .map { hints.hints(matching: .anyCabal(what: $0)) }
        await refresher.observe(streams)
    }

    public func setVisible(_ visible: Bool) {
        if !visible, cards.contains(where: \.isCommitted) { hasLeftHome = true }
        if visible { cards.removeAll { $0.isCommitted && isReleased($0) } }
        refresher.setVisible(visible)
    }

    private func apply(_ rows: [HomePendingVote]) {
        let listed = Set(rows.map(\.id))
        let existing = Dictionary(cards.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let kept = cards.filter { $0.isCommitted && !listed.contains($0.id) && !isReleased($0) }
        let fresh = rows.map { row -> HomeVoteCardModel in
            guard let card = existing[row.id] else {
                let created = HomeVoteCardModel(api: api, hints: hints, row: row, now: now, releaseAfter: releaseAfter)
                cardSetup?(created)
                return created
            }
            card.update(row)
            return card
        }
        cards = kept + fresh
        if !cards.contains(where: \.isCommitted) { hasLeftHome = false }
    }

    private func isReleased(_ card: HomeVoteCardModel) -> Bool {
        guard hasLeftHome, let releaseAt = card.releaseAt() else { return false }
        return now() >= releaseAt
    }
}

@MainActor
private final class HomePendingVotesReloadHook {
    var run: (@MainActor () async -> Void)?
}
