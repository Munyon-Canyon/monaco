import Foundation
import MonacoAPI
import Observation

public enum LeaderboardBoard: Hashable, Sendable {
    case cabals
    case people
    case cabalMembers(id: String)
}

public enum LeaderboardFilter: Hashable, Sendable {
    case everyone
    case friends
}

public struct LeaderboardQuery: Hashable, Sendable {
    public var board: LeaderboardBoard
    public var range: LeaderboardRange
    public var filter: LeaderboardFilter
}

@Observable
@MainActor
public final class LeaderboardLoader {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case loaded
        case failed(APIError)
    }

    public private(set) var query: LeaderboardQuery
    public private(set) var computedAt: Date?
    public private(set) var moves: [String: Int] = [:]
    public private(set) var toast: String?

    private let api: APIClient
    private let hints: any HintSource
    private var me: LeaderboardRowView?
    private var loadTask: Task<Void, Never>?

    @ObservationIgnored private lazy var pager = CursorPager<LeaderboardRowView> { [weak self, api] cursor in
        guard let query = await self?.query else { throw CancellationError() }
        do {
            let page = try await Self.page(query, cursor: cursor, api: api)
            await self?.notePage(page, for: query)
            return (page.rows.map(LeaderboardRowView.init), page.nextCursor)
        } catch {
            if !Task.isCancelled { await self?.noteFailure(APIError(error), for: query) }
            throw error
        }
    }

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(
        board: LeaderboardBoard, range: LeaderboardRange = .all, filter: LeaderboardFilter = .everyone,
        api: APIClient, hints: any HintSource
    ) {
        query = LeaderboardQuery(board: board, range: range, filter: filter)
        self.api = api
        self.hints = hints
    }

    public var range: LeaderboardRange { query.range }
    public var filter: LeaderboardFilter { query.filter }

    public var rows: [LeaderboardRowView] {
        pager.items.map { row in
            var row = row
            row.isViewer = row.id == me?.id
            return row
        }
    }

    public var pinnedMe: LeaderboardRowView? {
        guard let me, !pager.items.contains(where: { $0.id == me.id }) else { return nil }
        return me
    }

    public var phase: Phase {
        guard pager.items.isEmpty else { return .loaded }
        return switch pager.phase {
        case .failed(let error): .failed(error)
        case .exhausted: pinnedMe == nil ? .empty : .loaded
        case .idle, .loadingFirst, .loadingMore: .loading
        }
    }

    public var isLoading: Bool { pager.phase == .loadingFirst }

    public var isLoadingMore: Bool { pager.phase == .loadingMore }

    public var hasMore: Bool {
        switch pager.phase {
        case .idle, .loadingMore: !pager.items.isEmpty
        case .loadingFirst, .exhausted, .failed: false
        }
    }

    public func freshness(now: Date) -> String? {
        computedAt.map { freshnessLabel(computedAt: $0, now: now) }
    }

    public func load() async {
        if pager.items.isEmpty {
            await pager.loadFirst()
        } else {
            await refresh()
        }
    }

    public func loadMore() async {
        await pager.loadMore()
    }

    public func select(range: LeaderboardRange) {
        guard range != query.range || isFailed else { return }
        query.range = range
        replaceRows()
    }

    public func select(filter: LeaderboardFilter) {
        guard filter != query.filter || isFailed else { return }
        query.filter = filter
        replaceRows()
    }

    public func retry() {
        replaceRows()
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "leaderboards_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }

    private var isFailed: Bool {
        if case .failed = pager.phase { return true }
        return false
    }

    private func replaceRows() {
        moves = [:]
        loadTask?.cancel()
        loadTask = Task { [weak self] in
            guard !Task.isCancelled else { return }
            await self?.pager.loadFirst()
        }
    }

    private func refresh() async {
        let depth = pager.items.count
        let before = pager.items.map(\.key)
        let issued = query
        await pager.loadFirst()
        while pager.items.count < depth, pager.phase == .idle, issued == query {
            let count = pager.items.count
            await pager.loadMore()
            if pager.items.count == count { break }
        }
        if issued == query { moves = rankChanges(old: before, new: pager.items.map(\.key)) }
    }

    private func notePage(_ page: Components.Schemas.LeaderboardPage, for fetched: LeaderboardQuery) {
        guard fetched == query else { return }
        computedAt = page.computedAt
        me = page.me.map(LeaderboardRowView.init(me:))
    }

    private func noteFailure(_ error: APIError, for fetched: LeaderboardQuery) {
        guard fetched == query, !pager.items.isEmpty else { return }
        toast = ToastCopy.message(for: error)
    }

    private nonisolated static func page(
        _ query: LeaderboardQuery, cursor: String?, api: APIClient
    ) async throws -> Components.Schemas.LeaderboardPage {
        try await api.read { client in
            switch query.board {
            case .cabals:
                try await client.getCabalsLeaderboard(query: .init(range: query.range.generated, cursor: cursor))
                    .ok.body.json
            case .people:
                try await client.getPeopleLeaderboard(
                    query: .init(
                        range: query.range.generated, cursor: cursor,
                        filter: query.filter == .friends ? .friends : .all)
                ).ok.body.json
            case .cabalMembers(let id):
                try await client.getCabalLeaderboard(
                    path: .init(id: id), query: .init(range: query.range.generated, cursor: cursor)
                ).ok.body.json
            }
        }
    }
}
