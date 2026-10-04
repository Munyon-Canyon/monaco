import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class AccountActivityModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case failed(APIError)
        case loaded
    }

    public private(set) var toast: String?

    private let api: APIClient
    private let hints: any HintSource
    private let clock: @Sendable () -> Date

    @ObservationIgnored private lazy var pager = CursorPager<AccountActivityRow> { [weak self, api, clock] cursor in
        do {
            let page = try await api.read { client in
                try await client.getMyTxns(query: .init(limit: 30, cursor: cursor)).ok.body.json
            }
            let now = clock()
            return (try page.items.map { try AccountActivityRow($0, now: now) }, page.nextCursor)
        } catch {
            if !Task.isCancelled { await self?.noteFailure(APIError(error)) }
            throw error
        }
    }

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(api: APIClient, hints: any HintSource, clock: @escaping @Sendable () -> Date) {
        self.api = api
        self.hints = hints
        self.clock = clock
    }

    public var rows: [AccountActivityRow] { pager.items }

    public var isLoadingMore: Bool { pager.phase == .loadingMore }

    public var phase: Phase {
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
        await refresher.observe(hints.hints(matching: .user(what: "balance_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        toast = nil
    }

    private func noteFailure(_ error: APIError) {
        if !pager.items.isEmpty { toast = ToastCopy.message(for: error) }
    }
}
