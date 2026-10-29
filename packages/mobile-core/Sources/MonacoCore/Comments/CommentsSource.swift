import Foundation
import MonacoAPI

public struct CommentsPage: Sendable {
    public let threads: [Components.Schemas.CommentThread]
    public let nextCursor: String?

    public init(threads: [Components.Schemas.CommentThread], nextCursor: String?) {
        self.threads = threads
        self.nextCursor = nextCursor
    }
}

public enum ReplyRule: Equatable, Sendable {
    case anyone
    case members(cabalName: String, cabalPictureURL: String?)
}

public struct CommentPermissions: Equatable, Sendable {
    public let canComment: Bool
    public let canReact: Bool
    public let replyRule: ReplyRule?

    public init(canComment: Bool, canReact: Bool = true, replyRule: ReplyRule? = nil) {
        self.canComment = canComment
        self.canReact = canReact
        self.replyRule = replyRule
    }
}

public protocol CommentsSource: Sendable {
    func list(cursor: String?) async throws -> CommentsPage
    func post(
        body: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId: String?,
        submission: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment
    func delete(id: String, submission: IdempotentSubmission) async throws
    func permissions() async throws -> CommentPermissions
}

public struct FeedCommentsSource: CommentsSource {
    public let itemID: String
    private let api: APIClient

    public init(itemID: String, api: APIClient) {
        self.itemID = itemID
        self.api = api
    }

    public func list(cursor: String?) async throws -> CommentsPage {
        let page = try await api.read { [itemID] client in
            try await client.getFeedComments(path: .init(id: itemID), query: .init(cursor: cursor)).ok.body.json
        }
        return CommentsPage(threads: page.items, nextCursor: page.nextCursor)
    }

    public func post(
        body: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId: String?,
        submission: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        let request = Components.Schemas.CreateCommentRequest(
            body: body, attachment: attachment, parentCommentId: parentId)
        return try await api.submit(submission, payload: request, operation: "postFeedComment/\(itemID)") {
            [itemID] client, key in
            try await client.postFeedComment(
                path: .init(id: itemID), headers: .init(idempotencyKey: key), body: .json(request)
            ).created.body.json
        }
    }

    public func delete(id: String, submission: IdempotentSubmission) async throws {
        try await api.deleteComment(id: id, submission: submission)
    }

    public func permissions() async throws -> CommentPermissions {
        let item = try await api.read { [itemID] client in
            try await client.getFeedItem(path: .init(id: itemID)).ok.body.json.item
        }
        let rule: ReplyRule
        switch item.replyRule {
        case .anyone: rule = .anyone
        case .members:
            let name = item.replyRuleCabalName ?? item.cabalName ?? "this cabal"
            rule = .members(cabalName: name, cabalPictureURL: item.cabalPictureUrl)
        }
        return CommentPermissions(canComment: item.canReply, canReact: item.canReact, replyRule: rule)
    }
}

struct FeedItemNotLoaded: Error {}

public actor ProposalCommentsSource: CommentsSource {
    public nonisolated let proposalID: String
    private let api: APIClient
    private var feedItemID: String?

    public init(proposalID: String, api: APIClient) {
        self.proposalID = proposalID
        self.api = api
    }

    public func list(cursor: String?) async throws -> CommentsPage {
        let page = try await api.read { [proposalID] client in
            try await client.getProposalComments(path: .init(id: proposalID), query: .init(cursor: cursor)).ok.body
                .json
        }
        feedItemID = page.feedObjectId
        return CommentsPage(threads: page.items, nextCursor: page.nextCursor)
    }

    public func post(
        body: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId: String?,
        submission: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        let request = Components.Schemas.CreateCommentRequest(
            body: body, attachment: attachment, parentCommentId: parentId)
        return try await api.submit(submission, payload: request, operation: "postProposalComment/\(proposalID)") {
            [proposalID] client, key in
            try await client.postProposalComment(
                path: .init(id: proposalID), headers: .init(idempotencyKey: key), body: .json(request)
            ).created.body.json
        }
    }

    public func delete(id: String, submission: IdempotentSubmission) async throws {
        try await api.deleteComment(id: id, submission: submission)
    }

    public func permissions() async throws -> CommentPermissions {
        guard let itemID = feedItemID else { throw FeedItemNotLoaded() }
        return try await FeedCommentsSource(itemID: itemID, api: api).permissions()
    }
}

extension APIClient {
    fileprivate func deleteComment(id: String, submission: IdempotentSubmission) async throws {
        try await submit(submission, payload: id, operation: "deleteFeedComment") { client, key in
            _ = try await client.deleteFeedComment(path: .init(commentId: id), headers: .init(idempotencyKey: key))
                .noContent
        }
    }
}
