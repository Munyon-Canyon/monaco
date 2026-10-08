import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class StocksTabModel {
    public enum Browse: CaseIterable, Hashable, Sendable {
        case all, popular, preIpo

        public var title: String {
            switch self {
            case .all: "All"
            case .popular: "Popular"
            case .preIpo: "Pre-IPO"
            }
        }
    }
    public enum Section: Equatable, Sendable { case popular, preIpo, all, search }
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
    public private(set) var popular = SectionState()
    public private(set) var preIpo = SectionState()
    public private(set) var all = SectionState()
    public private(set) var search = SectionState()
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public private(set) var isLoadingMore = false
    public private(set) var browse: Browse = .all
    public var rows: [MarketAsset] { isSearching ? search.rows : self[browsedSection].rows }
    public var phase: Phase { isSearching ? search.phase : self[browsedSection].phase }
    public var nextCursor: String? { self[isSearching ? .search : browsedSection].nextCursor }
    public var hasMore: Bool { nextCursor != nil }
    public var isSearching: Bool { !Self.trimmed(query).isEmpty }
    public var loadMoreFailed: Bool { false }
    public var refreshFailed: Bool { lastError != nil }
    private let api: APIClient
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    private let refresher: HintRefresher
    private var generation = 0
    private var searchTask: Task<Void, Never>?
    private var replacementGenerations: Set<Int> = []

    public init(api: APIClient, hints: any HintSource, clock: any Clock<Duration>) {
        self.api = api
        self.hints = hints
        self.clock = clock
        let hook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.refreshPrices() }
    }
    public func load() async {
        generation += 1
        let issued = generation
        replacementGenerations.insert(issued)
        defer { replacementGenerations.remove(issued) }
        await replace(isSearching ? .search : browsedSection, issued: issued)
    }
    public func show(_ next: Browse) async {
        searchTask?.cancel()
        searchTask = nil
        browse = next
        if isSearching { search = .init(phase: .loading) }
        await load()
    }
    public func setQuery(_ text: String) {
        query = text
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
        await loadMore(isSearching ? .search : browsedSection)
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
        await refresh(isSearching ? .search : browsedSection, issued: issued)
    }
    private func replace(_ section: Section?, issued: Int) async {
        guard issued == generation else { return }
        self[section].phase = .loading
        do {
            let page = try await page(section, cursor: nil)
            guard issued == generation else { return }
            self[section] = .init(rows: page.assets, phase: .loaded, nextCursor: page.nextCursor, cursors: [nil])
        } catch {
            guard issued == generation else { return }
            note(APIError(error), section: section, empty: self[section].rows.isEmpty)
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
        } catch {
            guard issued == generation else { return }
            note(APIError(error), section: section, empty: self[section].rows.isEmpty)
        }
    }
    private func page(_ section: Section?, cursor: String?) async throws -> MarketAssetPage {
        let searchQuery = section == .search ? Self.trimmed(query) : nil
        let filter: Operations.GetAssets.Input.Query.FilterPayload =
            switch browse {
            case .all: .all
            case .popular: .popular
            case .preIpo: .preIpo
            }
        let list = try await api.read { client in
            try await client.getAssets(
                query: .init(
                    q: searchQuery, filter: filter, limit: Self.pageLimit,
                    cursor: cursor
                )
            ).ok.body.json
        }
        return MarketMapping.page(list)
    }
    private var browsedSection: Section {
        switch browse {
        case .all: .all
        case .popular: .popular
        case .preIpo: .preIpo
        }
    }
    private subscript(section: Section?) -> SectionState {
        get {
            switch section {
            case .popular: popular
            case .preIpo: preIpo
            case .all: all
            case .search: search
            case nil: search
            }
        }
        set {
            switch section {
            case .popular: popular = newValue
            case .preIpo: preIpo = newValue
            case .all: all = newValue
            case .search: search = newValue
            case nil: search = newValue
            }
        }
    }
    private func note(_ error: APIError, section: Section?, empty: Bool) {
        lastError = error
        if !empty { failureTick += 1 }
        self[section].phase = empty ? .failed(error) : .loaded
    }
    private static func trimmed(_ text: String) -> String { text.trimmingCharacters(in: .whitespacesAndNewlines) }
}
private final class ReloadHook { var run: (@MainActor () async -> Void)? }
