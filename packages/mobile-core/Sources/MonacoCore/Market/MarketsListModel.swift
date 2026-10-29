import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class MarketsListModel {
    public enum Order: CaseIterable, Hashable, Sendable {
        case name, price, movers

        public var title: String {
            switch self {
            case .name: "Name"
            case .price: "Price"
            case .movers: "Today's movers"
            }
        }

        public var identifier: String {
            switch self {
            case .name: "name"
            case .price: "price"
            case .movers: "movers"
            }
        }
    }

    public static let pageLimit = 20

    public private(set) var order: Order
    public private(set) var rows: [MarketAsset] = []
    public private(set) var phase: StocksTabModel.Phase = .idle
    public private(set) var nextCursor: String?
    public private(set) var isLoadingMore = false
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private var generation = 0

    public init(api: APIClient, order: Order) {
        self.api = api
        self.order = order
    }

    public func load() async {
        generation += 1
        let issued = generation
        phase = .loading
        do {
            let page = try await page(cursor: nil)
            guard issued == generation else { return }
            rows = page.assets
            nextCursor = page.nextCursor
            phase = .loaded
        } catch {
            guard issued == generation else { return }
            fail(APIError(error), empty: rows.isEmpty)
        }
    }

    public func select(_ next: Order) async {
        guard next != order else { return }
        order = next
        rows = []
        nextCursor = nil
        isLoadingMore = false
        await load()
    }

    public func loadMore() async {
        guard let cursor = nextCursor, !isLoadingMore, phase == .loaded else { return }
        let issued = generation
        isLoadingMore = true
        defer { isLoadingMore = false }
        do {
            let page = try await page(cursor: cursor)
            guard issued == generation else { return }
            rows += page.assets
            nextCursor = page.nextCursor
        } catch {
            guard issued == generation else { return }
            fail(APIError(error), empty: false)
        }
    }

    private func fail(_ error: APIError, empty: Bool) {
        lastError = error
        if empty {
            phase = .failed(error)
        } else {
            phase = .loaded
            failureTick += 1
        }
    }

    private func page(cursor: String?) async throws -> MarketAssetPage {
        let query = query(cursor: cursor)
        let list = try await api.read { client in try await client.getAssets(query: query).ok.body.json }
        return MarketMapping.page(list)
    }

    private func query(cursor: String?) -> Operations.GetAssets.Input.Query {
        switch order {
        case .name: .init(filter: .all, sort: .name, limit: Self.pageLimit, cursor: cursor)
        case .price: .init(filter: .all, sort: .price, limit: Self.pageLimit, cursor: cursor)
        case .movers: .init(filter: .movers, limit: Self.pageLimit, cursor: cursor)
        }
    }
}
