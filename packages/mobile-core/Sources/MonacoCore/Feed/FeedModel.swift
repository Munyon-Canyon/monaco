import Foundation
import MonacoAPI
import Observation

extension Components.Schemas.FeedItem: Identifiable {}

@Observable
@MainActor
public final class FeedModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty(query: String?)
        case followsNobody
        case failed(APIError)
        case loaded
    }

    public static let searchDebounce: Duration = .milliseconds(300)
    public static let pageSize = 30

    public private(set) var query = FeedQuery()
    public private(set) var pager: CursorPager<Components.Schemas.FeedItem>
    public private(set) var failureTick = 0
    public private(set) var lastError: APIError?
    private var viewerFollowsNobody = false
    private var checkingFollows = false

    private let api: APIClient
    private let viewerID: String?
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    @ObservationIgnored private var typedSearch = ""
    @ObservationIgnored private var searchTask: Task<Void, Never>?
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(api: APIClient, viewerID: String?, hints: any HintSource, clock: any Clock<Duration>) {
        self.api = api
        self.viewerID = viewerID
        self.hints = hints
        self.clock = clock
        pager = Self.pager(api: api, query: FeedQuery())
    }

    public var items: [Components.Schemas.FeedItem] { pager.items }

    public var isLoadingMore: Bool { pager.phase == .loadingMore }

    public var phase: Phase {
        guard pager.items.isEmpty else { return .loaded }
        switch pager.phase {
        case .failed(let error): return .failed(error)
        case .exhausted:
            if checkingFollows { return .loading }
            if query.scope == .following && viewerFollowsNobody { return .followsNobody }
            let search = query.trimmedSearch
            return .empty(query: search.isEmpty ? nil : search)
        case .idle, .loadingFirst, .loadingMore: return .loading
        }
    }

    public func select(_ chip: FeedChip) async {
        var next = query
        next.chip = chip
        next.search = typedSearch
        searchTask?.cancel()
        await apply(next)
    }

    public func select(_ scope: FeedScope) async {
        var next = query
        next.scope = scope
        next.search = typedSearch
        searchTask?.cancel()
        await apply(next)
    }

    public func setSearch(_ text: String) {
        typedSearch = text
        searchTask?.cancel()
        var next = query
        next.search = text
        guard next.trimmedSearch != query.trimmedSearch else { return }
        searchTask = Task { [weak self, clock] in
            do {
                try await clock.sleep(for: Self.searchDebounce)
            } catch {
                return
            }
            guard !Task.isCancelled else { return }
            await self?.apply(next)
        }
    }

    public func load() async {
        if pager.items.isEmpty {
            await reload()
        } else {
            await refresh()
        }
    }

    public func reload() async {
        let pager = pager
        await pager.loadFirst()
        noteFailure(of: pager)
        await checkFollows(after: pager)
    }

    public func refresh() async {
        let pager = pager
        await pager.refreshFirstPage()
        noteFailure(of: pager)
        await checkFollows(after: pager)
    }

    public func loadMore() async {
        let pager = pager
        await pager.loadMore()
        noteFailure(of: pager)
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "feed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func apply(_ next: FeedQuery) async {
        query = next
        pager = Self.pager(api: api, query: next)
        await reload()
    }

    private func checkFollows(after pager: CursorPager<Components.Schemas.FeedItem>) async {
        guard pager === self.pager, query.scope == .following, pager.phase == .exhausted, pager.items.isEmpty,
            let viewerID
        else { return }
        checkingFollows = true
        defer { checkingFollows = false }
        let profile = try? await api.read { client in
            try await client.getUser(path: .init(id: viewerID)).ok.body.json
        }
        viewerFollowsNobody = profile?.followingCount == 0
    }

    private func noteFailure(of pager: CursorPager<Components.Schemas.FeedItem>) {
        guard pager === self.pager, case .failed(let error) = pager.phase else { return }
        lastError = error
        if !pager.items.isEmpty { failureTick += 1 }
    }

    private static func pager(api: APIClient, query: FeedQuery) -> CursorPager<Components.Schemas.FeedItem> {
        CursorPager { cursor in
            let page = try await api.read { client in
                try await client.getFeed(query: query.parameters(cursor: cursor, limit: pageSize)).ok.body.json
            }
            return (page.items, page.nextCursor)
        }
    }
}
