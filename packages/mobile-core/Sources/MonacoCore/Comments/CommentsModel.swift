import MonacoAPI
import Observation

@Observable
@MainActor
public final class CommentsModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case loaded
        case failed(APIError)
    }

    public enum Notice: Equatable, Sendable {
        case failure(APIError)
        case deleted
    }

    public static let pendingRetryDelay: Duration = .seconds(1)
    public static let pageSize = 50

    public private(set) var canComment = false
    public private(set) var canReact = true
    public private(set) var replyRule: ReplyRule?
    public private(set) var isPosting = false
    public private(set) var lastPostedID: String?
    public private(set) var canCommentKnown = false
    public private(set) var canCommentFailed = false
    public private(set) var replySheet: ReplyTarget?
    public var isDockedFocused = false
    public private(set) var draft = ""
    public private(set) var sendFailed = false
    public private(set) var attachmentDraft: CommentAttachmentDraft?
    public private(set) var notice: Notice?
    public private(set) var noticeTick = 0
    public internal(set) var expandedRoots: Set<String> = []
    public internal(set) var highlightedID: String?
    public internal(set) var scrollRequest: ScrollRequest?

    public struct ScrollRequest: Equatable, Sendable {
        public let id: String
        public let tick: Int
    }

    public static let highlightDuration: Duration = .milliseconds(1200)

    private let source: any CommentsSource
    private let hints: any HintSource
    let clock: any Clock<Duration>
    private let onPosted: (@MainActor () -> Void)?
    let reactions: ReactionsModel?
    @ObservationIgnored var highlightTask: Task<Void, Never>?
    let pager: CursorPager<Components.Schemas.CommentThread>
    @ObservationIgnored private let postSubmission = IdempotentSubmission()
    @ObservationIgnored private var deleteSubmissions: [String: IdempotentSubmission] = [:]
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(
        source: any CommentsSource, hints: any HintSource, clock: any Clock<Duration>,
        reactions: ReactionsModel? = nil, onPosted: (@MainActor () -> Void)? = nil
    ) {
        self.source = source
        self.hints = hints
        self.clock = clock
        self.reactions = reactions
        self.onPosted = onPosted
        pager = CursorPager { cursor in
            let page = try await Self.retryingPending(clock: clock) { try await source.list(cursor: cursor) }
            return (page.threads, page.nextCursor)
        }
    }

    public static func forPost(
        item: Components.Schemas.FeedItem, api: APIClient, hints: any HintSource, clock: any Clock<Duration>,
        onPosted: (@MainActor () -> Void)? = nil
    ) -> CommentsModel {
        CommentsModel(
            source: FeedCommentsSource(itemID: item.threadItemId, api: api), hints: hints, clock: clock,
            reactions: ReactionsModel(api: api, clock: clock), onPosted: onPosted)
    }

    public var rows: [CommentThreadRow] { CommentThreadRow.rows(from: pager.items) }

    public var hasMore: Bool { pager.phase != .exhausted && !pager.items.isEmpty }

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
            await refreshFirstPage()
        }
        await loadCanComment()
    }

    public func refresh() async {
        await refreshFirstPage()
        await loadCanComment()
    }

    private func refreshFirstPage() async {
        await pager.refreshFirstPage()
        noteRefreshFailure()
    }

    public func loadMore() async {
        await pager.loadMore()
        noteRefreshFailure()
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "feed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func edit(_ text: String) {
        draft = text
        sendFailed = false
    }

    public func pick(_ next: CommentAttachmentDraft) -> String? {
        let replaced = attachmentDraft.map { CommentsCopy.replacedAttachment($0.kindName) }
        attachmentDraft = next
        sendFailed = false
        return replaced
    }

    public func removeAttachment() {
        attachmentDraft = nil
    }

    public func focusDocked() {
        replySheet = nil
        isDockedFocused = true
    }

    public func presentReplySheet(_ target: ReplyTarget) {
        if case .comment(let row) = target, row.isDeleted { return }
        isDockedFocused = false
        replySheet = target
    }

    public func dismissReplySheet() {
        replySheet = nil
    }

    public func post(
        _ draft: CommentDraft, attachment: Components.Schemas.CommentAttachmentInput? = nil
    ) async -> Bool {
        let body = draft.body
        let attachment = attachment ?? attachmentDraft?.input
        if case .tooLong = draft { return false }
        guard body != nil || attachment != nil, !isPosting else { return false }
        isPosting = true
        sendFailed = false
        defer { isPosting = false }
        let target = replySheet
        let parentID = target?.parentCommentID
        let created: Components.Schemas.Comment
        do {
            created = try await Self.retryingPending(clock: clock) {
                try await source.post(
                    body: body, attachment: attachment, parentId: parentID, submission: postSubmission)
            }
        } catch {
            sendFailed = true
            fail(APIError(error))
            return false
        }
        replySheet = nil
        attachmentDraft = nil
        self.draft = CommentDraft.field(self.draft, afterPosting: body ?? "")
        showPosted(created)
        if let rootID = created.parentCommentId { expandedRoots.insert(rootID) }
        if case .post = target {} else { await refresh() }
        lastPostedID = created.id
        onPosted?()
        return true
    }

    public func delete(_ row: CommentThreadRow) async {
        guard row.comment.isMine, !row.isDeleted else { return }
        let submission = deleteSubmissions[row.id] ?? IdempotentSubmission()
        deleteSubmissions[row.id] = submission
        do {
            try await source.delete(id: row.id, submission: submission)
        } catch {
            fail(APIError(error))
            return
        }
        deleteSubmissions[row.id] = nil
        if case .comment(let target) = replySheet, target.id == row.id { replySheet = nil }
        say(.deleted)
        showDeleted(row)
        await refresh()
    }

    private func showPosted(_ created: Components.Schemas.Comment) {
        if let threadID = created.parentCommentId {
            pager.update(id: threadID) { thread in
                if !thread.replies.contains(where: { $0.id == created.id }) { thread.replies.append(created) }
            }
        } else if pager.phase == .exhausted {
            pager.append(Components.Schemas.CommentThread(comment: created, replies: []))
        }
    }

    private func showDeleted(_ row: CommentThreadRow) {
        pager.update(id: row.comment.parentCommentId ?? row.id) { thread in
            if thread.comment.id == row.id { thread.comment.blank() }
            for index in thread.replies.indices where thread.replies[index].id == row.id {
                thread.replies[index].blank()
            }
        }
    }

    public func retryCanComment() async {
        await loadCanComment()
    }

    private func loadCanComment() async {
        guard !canCommentKnown else { return }
        do {
            let permissions = try await source.permissions()
            canComment = permissions.canComment
            canReact = permissions.canReact
            replyRule = permissions.replyRule
            canCommentKnown = true
            canCommentFailed = false
        } catch is FeedItemNotLoaded {
            return
        } catch {
            if !Task.isCancelled { canCommentFailed = true }
        }
    }

    private func noteRefreshFailure() {
        guard case .failed(let error) = pager.phase, !pager.items.isEmpty else { return }
        fail(error)
    }

    private func fail(_ error: APIError) {
        say(.failure(error))
    }

    private func say(_ notice: Notice) {
        self.notice = notice
        noticeTick += 1
    }

    private nonisolated static func retryingPending<T: Sendable>(
        clock: any Clock<Duration>, _ call: @Sendable () async throws -> T
    ) async throws -> T {
        do {
            return try await call()
        } catch {
            guard case .problem(let problem) = APIError(error), problem.code == .known(.feedItemPending) else {
                throw error
            }
            try await clock.sleep(for: pendingRetryDelay)
            return try await call()
        }
    }
}

extension Components.Schemas.Comment {
    fileprivate mutating func blank() {
        body = nil
        attachment = nil
        reactions = []
        reactionCount = 0
        bodyDisplay = CommentsCopy.deleted
    }

}
