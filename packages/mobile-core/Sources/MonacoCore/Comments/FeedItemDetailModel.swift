import MonacoAPI
import Observation

@Observable
@MainActor
public final class FeedItemDetailModel {
    public static let anchorPageLimit = 10
    public static let anchorMissing = "That reply was deleted."

    public let itemID: String
    public let anchorCommentID: String?
    public let comments: CommentsModel
    public private(set) var item: Components.Schemas.FeedItem?
    public private(set) var itemError: APIError?
    public private(set) var anchorMissingTick = 0

    private let api: APIClient
    private let hints: any HintSource
    private let threadID = ThreadItemID()
    @ObservationIgnored private var anchorSettled = false
    @ObservationIgnored private lazy var refresher = HintRefresher(minInterval: HintRefresher.feedInterval) {
        [weak self] in await self?.refresh()
    }

    public init(
        itemID: String, anchorCommentID: String? = nil, api: APIClient, hints: any HintSource,
        clock: any Clock<Duration>
    ) {
        self.itemID = itemID
        self.anchorCommentID = anchorCommentID
        self.api = api
        self.hints = hints
        comments = CommentsModel(
            source: PostCommentsSource(itemID: itemID, thread: threadID, api: api), hints: hints, clock: clock,
            reactions: ReactionsModel(api: api, clock: clock))
    }

    public func load() async {
        await loadItem()
        await comments.load()
        await settleAnchor()
    }

    public func refresh() async {
        async let header: Void = loadItem()
        await comments.refresh()
        await header
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .global(what: "feed")),
            hints.hints(matching: .anyCabal(what: "feed_item")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func reply() {
        comments.focusDocked()
    }

    private func settleAnchor() async {
        guard let anchor = anchorCommentID, !anchorSettled, comments.phase == .loaded || comments.phase == .empty
        else { return }
        anchorSettled = true
        var pages = 0
        while comments.row(id: anchor) == nil, comments.hasMore, pages < Self.anchorPageLimit {
            let before = comments.rows.count
            await comments.loadMore()
            pages += 1
            if comments.rows.count == before { break }
        }
        guard let row = comments.row(id: anchor), !row.isDeleted else {
            anchorMissingTick += 1
            return
        }
        comments.jump(to: row.asQuote)
    }

    private func loadItem() async {
        do {
            let detail = try await api.read { [itemID] client in
                try await client.getFeedItem(path: .init(id: itemID)).ok.body.json
            }
            item = detail.item
            itemError = nil
            await threadID.set(detail.item.threadItemId)
        } catch {
            if !Task.isCancelled, item == nil { itemError = APIError(error) }
        }
    }
}

actor ThreadItemID {
    private var id: String?

    func set(_ id: String) {
        self.id = id
    }

    func resolved(or fallback: String) -> String {
        id ?? fallback
    }
}

struct PostCommentsSource: CommentsSource {
    let itemID: String
    let thread: ThreadItemID
    let api: APIClient

    private func source() async -> FeedCommentsSource {
        FeedCommentsSource(itemID: await thread.resolved(or: itemID), api: api)
    }

    func list(cursor: String?) async throws -> CommentsPage {
        try await source().list(cursor: cursor)
    }

    func post(
        body: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId: String?,
        submission: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        try await source().post(body: body, attachment: attachment, parentId: parentId, submission: submission)
    }

    func delete(id: String, submission: IdempotentSubmission) async throws {
        try await source().delete(id: id, submission: submission)
    }

    func permissions() async throws -> CommentPermissions {
        try await source().permissions()
    }
}

extension CommentsModel {
    func row(id: String) -> CommentThreadRow? {
        rows.first { $0.id == id }
    }
}

extension CommentThreadRow {
    fileprivate var asQuote: CommentQuoteRow {
        CommentQuoteRow(
            Components.Schemas.CommentQuote(
                commentId: id, rootCommentId: comment.parentCommentId ?? id, authorDisplayName: authorName,
                excerpt: nil, isDeleted: false, hasAttachment: false, isFolded: isFolded))
    }
}
