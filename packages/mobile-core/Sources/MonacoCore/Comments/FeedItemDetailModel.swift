import MonacoAPI
import Observation

@Observable
@MainActor
public final class FeedItemDetailModel {
    public let itemID: String
    public let comments: CommentsModel
    public private(set) var item: Components.Schemas.FeedItem?
    public private(set) var itemError: APIError?

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(itemID: String, api: APIClient, hints: any HintSource, clock: any Clock<Duration>) {
        self.itemID = itemID
        self.api = api
        self.hints = hints
        comments = CommentsModel(source: FeedCommentsSource(itemID: itemID, api: api), hints: hints, clock: clock)
    }

    public func load() async {
        async let header: Void = loadItem()
        await comments.load()
        await header
    }

    public func refresh() async {
        async let header: Void = loadItem()
        await comments.refresh()
        await header
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "feed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func loadItem() async {
        do {
            let detail = try await api.read { [itemID] client in
                try await client.getFeedItem(path: .init(id: itemID)).ok.body.json
            }
            item = detail.item
            itemError = nil
        } catch {
            if !Task.isCancelled, item == nil { itemError = APIError(error) }
        }
    }
}
