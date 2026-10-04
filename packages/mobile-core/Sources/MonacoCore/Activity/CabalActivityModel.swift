import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalActivityModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case loaded
        case failed(APIError)
        case hidden
    }

    public static let previewCount = 5

    public let cabalID: String
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0

    private let api: APIClient
    private let hints: any HintSource
    private let clock: @Sendable () -> Date
    private var notMember = false

    @ObservationIgnored private lazy var pager = CursorPager<ActivityRow> { [weak self, cabalID, api, clock] cursor in
        do {
            let page = try await Self.page(cabalID: cabalID, cursor: cursor, limit: 30, api: api)
            let now = clock()
            await self?.noteSuccess()
            return (page.items.map { ActivityRow($0, now: now) }, page.nextCursor)
        } catch {
            if !Task.isCancelled { await self?.noteFailure(APIError(error)) }
            throw error
        }
    }

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(cabalID: String, api: APIClient, hints: any HintSource, clock: @escaping @Sendable () -> Date) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
        self.clock = clock
    }

    public var rows: [ActivityRow] { pager.items }

    public var firstFive: [ActivityRow] { Array(pager.items.prefix(Self.previewCount)) }

    public var hasMore: Bool {
        pager.items.count > Self.previewCount || (!pager.items.isEmpty && pager.phase != .exhausted)
    }

    public var isLoadingMore: Bool { pager.phase == .loadingMore }

    public var phase: Phase {
        if notMember { return .hidden }
        guard pager.items.isEmpty else { return .loaded }
        return switch pager.phase {
        case .failed(let error): .failed(error)
        case .exhausted: .empty
        case .idle, .loadingFirst, .loadingMore: .loading
        }
    }

    public func load() async {
        if pager.items.isEmpty {
            await pager.loadFirst()
        } else {
            await refresh()
        }
    }

    public func refresh() async {
        await pager.refreshFirstPage()
    }

    public func loadMore() async {
        await pager.loadMore()
    }

    public func observe() async {
        await refresher.observe(
            ["activity_changed", "swap_updated"].map { hints.hints(matching: .cabal(id: cabalID, what: $0)) })
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func find(id: String) async -> ActivityRow? {
        if let row = pager.items.first(where: { $0.id == id }) { return row }
        var cursor: String?
        repeat {
            guard let page = try? await Self.page(cabalID: cabalID, cursor: cursor, limit: 100, api: api) else {
                return nil
            }
            if let match = page.items.first(where: { $0.id == id }) { return ActivityRow(match, now: clock()) }
            cursor = page.nextCursor
        } while cursor != nil
        return nil
    }

    private nonisolated static func page(
        cabalID: String, cursor: String?, limit: Int, api: APIClient
    ) async throws -> Components.Schemas.CabalActivityPage {
        try await api.read { client in
            try await client.getCabalActivity(path: .init(id: cabalID), query: .init(limit: limit, cursor: cursor))
                .ok.body.json
        }
    }

    private func noteSuccess() {
        notMember = false
    }

    private func noteFailure(_ error: APIError) {
        if case .problem(let problem) = error, problem.code == .known(.notCabalMember) {
            notMember = true
            return
        }
        guard !pager.items.isEmpty else { return }
        lastError = error
        failureTick += 1
    }
}
