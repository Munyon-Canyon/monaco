import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class StocksTabModel {
    public enum Browse: Equatable, Sendable {
        case popular
        case preIpo
    }
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed(APIError)
    }
    public static let searchDebounce: Duration = .milliseconds(300)
    public static let pageLimit = 20
    public private(set) var query = ""
    public private(set) var browse: Browse = .popular
    public private(set) var rows: [MarketAsset] = []
    public private(set) var phase: Phase = .idle
    public private(set) var nextCursor: String?
    public private(set) var isLoadingMore = false
    public private(set) var loadMoreFailed = false
    public private(set) var refreshFailed = false
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public var held: [MarketAsset] { [] }
    public var isSearching: Bool {
        !Self.trimmed(query).isEmpty
    }
    public var hasMore: Bool {
        nextCursor != nil
    }
    private let api: APIClient
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    private let refresher: HintRefresher
    private var generation = 0
    private var rowsCursor: String?
    private var pageCursors: [String?] = []
    private var isRefreshingPages = false
    private var searchTask: Task<Void, Never>?
    public init(api: APIClient, hints: any HintSource, clock: any Clock<Duration>) {
        self.api = api
        self.hints = hints
        self.clock = clock
        let hook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.refreshPrices()
        }
    }
    public func load() async {
        searchTask?.cancel()
        generation += 1
        await reload(generation: generation, preservingPages: false)
    }
    private func refreshPrices() async {
        generation += 1
        await reload(generation: generation, preservingPages: true)
    }
    public func show(_ next: Browse) async {
        browse = next
        query = ""
        beginReplacement()
        await load()
    }
    public func setQuery(_ text: String) {
        query = text
        beginReplacement()
        searchTask?.cancel()
        generation += 1
        let issued = generation
        let trimmed = Self.trimmed(text)
        guard !trimmed.isEmpty else {
            searchTask = Task { await self.reload(generation: issued, preservingPages: false) }
            return
        }
        if rows.isEmpty {
            phase = .loading
        }
        searchTask = Task { [weak self] in
            guard let self else { return }
            do {
                try await self.clock.sleep(for: Self.searchDebounce)
            } catch {
                return
            }
            guard !Task.isCancelled else { return }
            await self.reload(generation: issued, preservingPages: false)
        }
    }
    public func loadMore() async {
        guard let cursor = nextCursor, !isLoadingMore, !isRefreshingPages else { return }
        let issued = generation
        isLoadingMore = true
        defer { isLoadingMore = false }
        do {
            let page = try await page(cursor: cursor)
            guard issued == generation else { return }
            rows.append(contentsOf: page.assets)
            rowsCursor = page.nextCursor
            nextCursor = page.nextCursor
            pageCursors.append(cursor)
            loadMoreFailed = false
        } catch {
            guard issued == generation else { return }
            loadMoreFailed = true
            note(APIError(error), replacingRows: false)
        }
    }
    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "prices_updated")))
    }
    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
    private func beginReplacement() {
        rows = []
        nextCursor = nil
        rowsCursor = nil
        pageCursors = []
        refreshFailed = false
        loadMoreFailed = false
        phase = .loading
    }
    private func reload(generation issued: Int, preservingPages: Bool) async {
        guard issued == generation else { return }
        if preservingPages { isRefreshingPages = true }
        defer {
            if issued == generation { isRefreshingPages = false }
        }
        nextCursor = preservingPages ? rowsCursor : nil
        if rows.isEmpty {
            phase = .loading
        }
        loadMoreFailed = false
        do {
            let cursors = preservingPages && !pageCursors.isEmpty ? pageCursors : [nil]
            var pages: [MarketAssetPage] = []
            for cursor in cursors {
                pages.append(try await page(cursor: cursor))
            }
            guard issued == generation else { return }
            rows = pages.flatMap(\.assets)
            rowsCursor = pages.last?.nextCursor
            nextCursor = rowsCursor
            pageCursors = cursors
            phase = .loaded
            refreshFailed = false
        } catch {
            guard issued == generation else { return }
            if !rows.isEmpty {
                nextCursor = rowsCursor
            }
            note(APIError(error), replacingRows: true)
        }
    }
    private func page(cursor: String?) async throws -> MarketAssetPage {
        let trimmed = Self.trimmed(query)
        let searching = !trimmed.isEmpty
        let filter: Operations.GetAssets.Input.Query.FilterPayload? =
            searching ? nil : Self.wireFilter(browse)
        let list = try await api.read { client in
            try await client.getAssets(
                query: .init(
                    q: searching ? trimmed : nil,
                    filter: filter,
                    limit: Self.pageLimit,
                    cursor: cursor
                )
            ).ok.body.json
        }
        return MarketMapping.page(list)
    }
    private func note(_ error: APIError, replacingRows: Bool) {
        lastError = error
        failureTick += 1
        if replacingRows && rows.isEmpty {
            phase = .failed(error)
            nextCursor = nil
            return
        }
        refreshFailed = true
        if phase != .loaded {
            phase = .loaded
        }
    }
    private static func wireFilter(_ browse: Browse) -> Operations.GetAssets.Input.Query.FilterPayload {
        switch browse {
        case .popular: .popular
        case .preIpo: .preIpo
        }
    }
    private static func trimmed(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
private final class ReloadHook {
    var run: (@MainActor () async -> Void)?
}
