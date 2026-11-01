import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class StocksTabModel {
    public enum Browse: CaseIterable, Hashable, Sendable {
        case all, watchlist, preIpo, held

        public var title: String {
            switch self {
            case .all: "All"
            case .watchlist: "Watchlist"
            case .preIpo: "Pre-IPO"
            case .held: "Held"
            }
        }

        public var identifier: String {
            switch self {
            case .all: "all"
            case .watchlist: "watchlist"
            case .preIpo: "pre_ipo"
            case .held: "held"
            }
        }
    }
    public nonisolated static let availableBrowses: [Browse] = [.all, .watchlist, .preIpo, .held]
    public enum Section: Equatable, Sendable { case preIpo, all, search }
    public enum Phase: Equatable, Sendable {
        case idle, loading, loaded
        case failed(APIError)
    }
    public struct SectionState: Equatable, Sendable {
        public var rows: [MarketAsset] = []
        public var phase: Phase = .idle
        public var nextCursor: String?
        fileprivate var cursors: [String?] = []
    }
    public static let searchDebounce: Duration = .milliseconds(300)
    public static let pageLimit = 20
    public private(set) var query = ""
    public private(set) var preIpo = SectionState()
    public private(set) var all = SectionState()
    public private(set) var search = SectionState()
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public private(set) var isLoadingMore = false
    public private(set) var browse: Browse = .all
    public private(set) var lastLoadedAt: Date?
    public private(set) var isStale = false
    public internal(set) var facts: [String: MarketFact] = [:]
    public internal(set) var movers: [MarketAsset] = []
    public private(set) var isMarkets = false
    public let quotes: WatchlistQuotes?
    public var rows: [MarketAsset] { self[activeSection].rows }
    public var phase: Phase { self[activeSection].phase }
    public var nextCursor: String? { self[activeSection].nextCursor }
    public var hasMore: Bool { nextCursor != nil }
    public var searchResults: [MarketAsset] {
        search.rows.filter { $0.kind != .preIpo } + search.rows.filter { $0.kind == .preIpo }
    }
    public var isSearching: Bool { !Self.trimmed(query).isEmpty }
    public var loadMoreFailed: Bool { false }
    public var refreshFailed: Bool { lastError != nil }
    let api: APIClient
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    let now: @Sendable () -> Date
    private let refresher: HintRefresher
    let watchlist: WatchlistModel?
    private var generation = 0
    private var searchTask: Task<Void, Never>?
    private var replacementGenerations: Set<Int> = []

    public init(
        api: APIClient, hints: any HintSource, clock: any Clock<Duration>, watchlist: WatchlistModel? = nil,
        now: @escaping @Sendable () -> Date = WallClock.now
    ) {
        self.api = api
        self.hints = hints
        self.clock = clock
        self.now = now
        self.watchlist = watchlist
        quotes = watchlist.map { WatchlistQuotes(api: api, watchlist: $0) }
        let hook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.refreshPrices() }
    }
    public func load() async {
        generation += 1
        let issued = generation
        replacementGenerations.insert(issued)
        defer { replacementGenerations.remove(issued) }
        await replace(activeSection, issued: issued)
    }
    public func loadMarkets() async {
        isMarkets = true
        async let listed: Void = load()
        async let watched: Void = loadQuotes()
        async let known: Void = loadFacts()
        async let moving: Void = loadMovers()
        _ = await (listed, watched, known, moving)
    }
    public func show(_ next: Browse) async {
        browse = next
        guard !isSearching else { return }
        searchTask?.cancel()
        searchTask = nil
        isStale = false
        if !isMarkets { await load() }
    }
    public func searchLines(for asset: MarketAsset) -> (title: String, subline: String) {
        let name = asset.name.trimmingCharacters(in: .whitespacesAndNewlines)
        let ticker = asset.ticker
        guard name.count > ticker.count, name.lowercased().hasPrefix(ticker.lowercased()) else {
            return (name, ticker)
        }
        let trim = CharacterSet.whitespacesAndNewlines.union(.punctuationCharacters)
        let rest = name.dropFirst(ticker.count).trimmingCharacters(in: trim)
        return (ticker, rest.isEmpty ? name : rest)
    }
    public func setQuery(_ text: String) {
        query = text
        isStale = false
        searchTask?.cancel()
        generation += 1
        let issued = generation
        guard !Self.trimmed(text).isEmpty else {
            searchTask = Task { [weak self] in
                guard let self else { return }
                guard issued == self.generation else { return }
                self.searchTask = nil
                await self.load()
            }
            return
        }
        search = .init(phase: .loading)
        searchTask = Task { [weak self] in
            guard let self else { return }
            do { try await self.clock.sleep(for: Self.searchDebounce) } catch { return }
            guard !Task.isCancelled else { return }
            await self.replace(.search, issued: issued)
        }
    }
    public func loadMore() async {
        await loadMore(activeSection)
    }
    private func loadMore(_ section: Section) async {
        guard !replacementGenerations.contains(generation), let cursor = self[section].nextCursor, !isLoadingMore else {
            return
        }
        let issued = generation
        isLoadingMore = true
        defer { isLoadingMore = false }
        do {
            let page = try await page(section, cursor: cursor)
            guard issued == generation else { return }
            self[section].rows += page.assets
            self[section].nextCursor = page.nextCursor
            self[section].cursors.append(cursor)
        } catch {
            guard issued == generation else { return }
            note(APIError(error), section: section, empty: false)
        }
    }
    public func observe() async { await refresher.observe(hints.hints(matching: .global(what: "prices_updated"))) }
    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }
    private func refreshPrices() async {
        generation += 1
        let issued = generation
        replacementGenerations.insert(issued)
        defer { replacementGenerations.remove(issued) }
        async let moving: Void = isMarkets ? loadMovers() : ()
        await refresh(activeSection, issued: issued)
        await refreshWatchlist()
        await moving
    }
    private func replace(_ section: Section?, issued: Int) async {
        guard issued == generation else { return }
        self[section].phase = .loading
        do {
            let page = try await page(section, cursor: nil)
            guard issued == generation else { return }
            self[section] = .init(rows: page.assets, phase: .loaded, nextCursor: page.nextCursor, cursors: [nil])
            settle(section)
        } catch {
            guard issued == generation else { return }
            note(APIError(error), section: section, empty: self[section].rows.isEmpty, refreshing: true)
        }
    }
    private func refresh(_ section: Section?, issued: Int) async {
        guard issued == generation else { return }
        let cursors = self[section].cursors.isEmpty ? [nil] : self[section].cursors
        do {
            var pages: [MarketAssetPage] = []
            for cursor in cursors { pages.append(try await page(section, cursor: cursor)) }
            guard issued == generation else { return }
            self[section].rows = pages.flatMap(\.assets)
            self[section].nextCursor = pages.last?.nextCursor
            self[section].cursors = cursors
            self[section].phase = .loaded
            settle(section)
        } catch {
            guard issued == generation else { return }
            note(APIError(error), section: section, empty: self[section].rows.isEmpty, refreshing: true)
        }
    }
    private func page(_ section: Section?, cursor: String?) async throws -> MarketAssetPage {
        let searchQuery = section == .search ? Self.trimmed(query) : nil
        let filter: Operations.GetAssets.Input.Query.FilterPayload = section == .preIpo ? .preIpo : .all
        let list = try await api.read { client in
            try await client.getAssets(
                query: .init(q: searchQuery, filter: filter, limit: Self.pageLimit, cursor: cursor)
            ).ok.body.json
        }
        return MarketMapping.page(list)
    }
    private var browsedSection: Section {
        if isMarkets { return .preIpo }
        return browse == .preIpo ? .preIpo : .all
    }
    private var activeSection: Section { isSearching ? .search : browsedSection }
    private subscript(section: Section?) -> SectionState {
        get {
            switch section {
            case .preIpo: preIpo
            case .all: all
            case .search: search
            case nil: search
            }
        }
        set {
            switch section {
            case .preIpo: preIpo = newValue
            case .all: all = newValue
            case .search: search = newValue
            case nil: search = newValue
            }
        }
    }
    private func note(_ error: APIError, section: Section?, empty: Bool, refreshing: Bool = false) {
        lastError = error
        if !empty, !BackgroundRefresh.isActive { failureTick += 1 }
        self[section].phase = empty ? .failed(error) : .loaded
        if refreshing, !empty, section == currentSection { isStale = true }
    }
    private func settle(_ section: Section?) {
        guard section == currentSection else { return }
        lastLoadedAt = now()
        isStale = false
    }
    private var currentSection: Section { activeSection }
    func loadQuotes() async { await quotes?.load() }
    func loadMissingQuotes() async { await quotes?.loadMissing() }
    private static func trimmed(_ text: String) -> String { text.trimmingCharacters(in: .whitespacesAndNewlines) }
}
private final class ReloadHook { var run: (@MainActor () async -> Void)? }
