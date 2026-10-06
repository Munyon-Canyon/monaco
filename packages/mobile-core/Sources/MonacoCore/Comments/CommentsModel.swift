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

    public private(set) var canComment = true
    public private(set) var isPosting = false
    private var canCommentKnown = false
    public private(set) var replyTarget: CommentThreadRow?
    public private(set) var notice: Notice?
    public private(set) var noticeTick = 0

    private let source: any CommentsSource
    private let hints: any HintSource
    private let clock: any Clock<Duration>
    private let pager: CursorPager<Components.Schemas.CommentThread>
    @ObservationIgnored private let postSubmission = IdempotentSubmission()
    @ObservationIgnored private var deleteSubmissions: [String: IdempotentSubmission] = [:]
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(source: any CommentsSource, hints: any HintSource, clock: any Clock<Duration>) {
        self.source = source
        self.hints = hints
        self.clock = clock
        pager = CursorPager { cursor in
            let page = try await Self.retryingPending(clock: clock) { try await source.list(cursor: cursor) }
            return (page.threads, page.nextCursor)
        }
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
            await refresh()
        }
        await loadCanComment()
    }

    public func refresh() async {
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

    public func beginReply(to row: CommentThreadRow) {
        guard !row.isDeleted else { return }
        replyTarget = row
    }

    public func cancelReply() {
        replyTarget = nil
    }

    public func post(_ text: String) async -> Bool {
        guard let body = CommentDraft(text: text).body, !isPosting else { return false }
        isPosting = true
        defer { isPosting = false }
        let parentID = replyTarget?.id
        do {
            _ = try await Self.retryingPending(clock: clock) {
                try await source.post(body: body, parentId: parentID, submission: postSubmission)
            }
        } catch {
            fail(APIError(error))
            return false
        }
        replyTarget = nil
        await refresh()
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
        if replyTarget?.id == row.id { replyTarget = nil }
        say(.deleted)
        await refresh()
    }

    private func loadCanComment() async {
        guard !canCommentKnown else { return }
        guard let allowed = try? await source.canComment() else { return }
        canComment = allowed
        canCommentKnown = true
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
