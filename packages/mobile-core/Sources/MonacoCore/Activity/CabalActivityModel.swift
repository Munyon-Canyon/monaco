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

    public struct Toast: Equatable, Sendable {
        public let message: String
        public let isSuccess: Bool
        let tick: Int
    }

    public enum ToastText {
        public static let retrying = "Retrying the trade"
        public static let bought = "Bought. Holdings updated"
        public static let sold = "Sold. Holdings updated"
        public static let failedAgain = "It didn't go through again. Try later"
    }

    private struct Retry {
        let kind: ActivityRow.Kind
        let symbol: String?
        let seen: Set<String>
        var awaitingFetch = true
    }

    public static let previewCount = 5

    public let cabalID: String
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public private(set) var toast: Toast?
    public private(set) var openSwap: SwapReceipt?

    private let api: APIClient
    private let hints: any HintSource
    private let clock: @Sendable () -> Date
    private var notMember = false
    private var retries: [String: Retry] = [:]
    private var superseded: Set<String> = []
    private var openSwapID: String?
    @ObservationIgnored private var submissions: [String: IdempotentSubmission] = [:]

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

    public var rows: [ActivityRow] { pager.items.map(present) }

    public var firstFive: [ActivityRow] { pager.items.prefix(Self.previewCount).map(present) }

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
            settleRetries()
        } else {
            await refresh()
        }
    }

    public func refresh() async {
        await pager.refreshFirstPage()
        settleRetries()
        if let openSwapID { await loadSwap(id: openSwapID) }
    }

    public func loadSwap(id: String) async {
        openSwapID = id
        do {
            let swap = try await api.read { client in try await client.getSwap(path: .init(id: id)).ok.body.json }
            openSwap = SwapReceipt(swap)
        } catch {
            if !Task.isCancelled, openSwap != nil { noteFailure(APIError(error)) }
        }
    }

    public func retrySwap(_ row: ActivityRow) async {
        guard row.kind.isSwap, retries[row.id] == nil else { return }
        let submission = submissions[row.id] ?? IdempotentSubmission()
        submissions[row.id] = submission
        do {
            _ = try await api.submit(submission, payload: row.id, operation: "postSwapRetry") { client, key in
                try await client.postSwapRetry(path: .init(id: row.id), headers: .init(idempotencyKey: key))
                    .accepted.body.json
            }
            retries[row.id] = Retry(kind: row.kind, symbol: row.symbol, seen: Set(pager.items.map(\.id)))
            show(ToastText.retrying, isSuccess: true)
        } catch {
            show(ToastCopy.message(for: APIError(error)), isSuccess: false)
        }
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

    private func present(_ row: ActivityRow) -> ActivityRow {
        var row = row
        if let retry = retries[row.id] {
            if retry.awaitingFetch { row.status = .pending }
            row.offersRetry = false
        } else if superseded.contains(row.id) {
            row.offersRetry = false
        }
        return row
    }

    private func settleRetries() {
        for (id, retry) in retries {
            retries[id]?.awaitingFetch = false
            let next = pager.items.first {
                !retry.seen.contains($0.id) && $0.kind == retry.kind && $0.symbol == retry.symbol
            }
            guard let next, next.status != .pending else { continue }
            retries[id] = nil
            superseded.insert(id)
            switch (next.status, retry.kind) {
            case (.failed, _): show(ToastText.failedAgain, isSuccess: false)
            case (_, .sell): show(ToastText.sold, isSuccess: true)
            default: show(ToastText.bought, isSuccess: true)
            }
        }
    }

    private func show(_ message: String, isSuccess: Bool) {
        toast = Toast(message: message, isSuccess: isSuccess, tick: (toast?.tick ?? 0) + 1)
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
