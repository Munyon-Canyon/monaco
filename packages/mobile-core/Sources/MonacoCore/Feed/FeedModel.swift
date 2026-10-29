import Foundation
import MonacoAPI
import Observation

extension Components.Schemas.FeedItem: Identifiable {}

@Observable
@MainActor
public final class FeedModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case followsNobody
        case failed(APIError)
        case loaded
    }

    public static let pageSize = 30

    public private(set) var query = FeedQuery()
    public private(set) var pager: CursorPager<Components.Schemas.FeedItem>
    public private(set) var failureTick = 0
    public private(set) var lastError: APIError?
    public private(set) var loadMoreFailed = false
    public private(set) var fromEveryone: [Components.Schemas.FeedItem] = []
    private var fromEveryoneLoaded = false
    private var likesInFlight: Set<String> = []
    private var viewerFollowsNobody = false
    private var checkingFollows = false

    private let api: APIClient
    private let mutes: FeedMuteService
    private let likes: FeedLikeService
    private let viewerID: String?
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    private let now: @Sendable () -> Date
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(
        api: APIClient, viewerID: String?, hints: any HintSource, clock: any Clock<Duration>,
        now: @escaping @Sendable () -> Date = Date.init
    ) {
        self.api = api
        mutes = FeedMuteService(api: api)
        likes = FeedLikeService(api: api)
        self.viewerID = viewerID
        self.hints = hints
        self.clock = clock
        self.now = now
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
            return .empty
        case .idle, .loadingFirst, .loadingMore: return .loading
        }
    }

    public static let quietAfter: TimeInterval = 7 * 24 * 60 * 60
    public static let othersLimit = 4

    public var isQuietWeek: Bool {
        guard query.scope == .network else { return false }
        guard let newest = pager.items.map(\.createdAt).max() else { return pager.phase == .exhausted }
        return now().timeIntervalSince(newest) > Self.quietAfter
    }

    public func select(_ scope: FeedScope) async {
        await apply(FeedQuery(scope: scope))
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
        loadMoreFailed = false
        noteFailure(of: pager)
        await checkFollows(after: pager)
        await syncFromEveryone(refreshing: false)
    }

    public func refresh() async {
        let pager = pager
        await pager.refreshFirstPage()
        if !pager.phase.isFailed { loadMoreFailed = false }
        noteFailure(of: pager)
        await checkFollows(after: pager)
        await syncFromEveryone(refreshing: true)
    }

    public func loadMore() async {
        let pager = pager
        let before = pager.phase
        await pager.loadMore()
        noteFailure(of: pager, raisesToast: false)
        guard pager === self.pager else { return }
        switch pager.phase {
        case .idle, .exhausted: loadMoreFailed = false
        case .failed: if !before.isFailed { loadMoreFailed = true }
        case .loadingFirst, .loadingMore: break
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "feed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func muteOptions(for item: Components.Schemas.FeedItem) -> [FeedMuteOption] {
        FeedMuteOption.options(for: item, viewerID: viewerID)
    }

    public func mute(_ option: FeedMuteOption) async -> FeedMuteReceipt? {
        do {
            try await mutes.mute(option.target)
        } catch {
            fail(APIError(error))
            return nil
        }
        pager.remove { FeedMuteMatcher.removes(option.target, $0) }
        if pager.items.isEmpty, pager.phase == .idle { await reload() }
        return FeedMuteReceipt(target: option.target, message: option.toast)
    }

    public func undo(_ receipt: FeedMuteReceipt) async {
        do {
            try await mutes.unmute(receipt.target)
        } catch {
            fail(APIError(error))
            return
        }
        await reload()
    }

    public func toggleLike(_ item: Components.Schemas.FeedItem) async {
        let id = item.id
        guard likesInFlight.insert(id).inserted else { return }
        defer { likesInFlight.remove(id) }
        let pager = pager
        let before = pager.items.first { $0.id == id } ?? item
        let liking = !before.likedByMe
        pager.update(id: id) { $0.setLike(liking, from: before) }
        do {
            try await likes.set(liking, itemID: id)
        } catch {
            pager.update(id: id) {
                $0.likedByMe = before.likedByMe
                $0.likeCount = before.likeCount
            }
            fail(APIError(error))
        }
    }

    private func fail(_ error: APIError) {
        lastError = error
        failureTick += 1
    }

    private func apply(_ next: FeedQuery) async {
        query = next
        let candidate = Self.pager(api: api, query: next)
        loadMoreFailed = false
        guard !pager.items.isEmpty else {
            pager = candidate
            await reload()
            return
        }
        await candidate.loadFirst()
        guard query == next else { return }
        pager = candidate
        noteFailure(of: candidate)
        await checkFollows(after: candidate)
        await syncFromEveryone(refreshing: false)
    }

    private func syncFromEveryone(refreshing: Bool) async {
        guard isQuietWeek else {
            fromEveryone = []
            fromEveryoneLoaded = false
            return
        }
        guard refreshing || !fromEveryoneLoaded else { return }
        fromEveryoneLoaded = true
        let others = FeedQuery(scope: .others)
        let page = try? await api.read { client in
            try await client.getFeed(query: others.parameters(cursor: nil, limit: Self.othersLimit)).ok.body.json
        }
        guard isQuietWeek else { return }
        fromEveryone = page?.items ?? []
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

    private func noteFailure(of pager: CursorPager<Components.Schemas.FeedItem>, raisesToast: Bool = true) {
        guard pager === self.pager, case .failed(let error) = pager.phase else { return }
        lastError = error
        if raisesToast, !pager.items.isEmpty, !BackgroundRefresh.isActive { failureTick += 1 }
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

public struct FeedMuteReceipt: Equatable, Sendable {
    public let target: FeedMuteTarget
    public let message: String
}

extension PagerPhase {
    fileprivate var isFailed: Bool {
        if case .failed = self { return true }
        return false
    }
}
