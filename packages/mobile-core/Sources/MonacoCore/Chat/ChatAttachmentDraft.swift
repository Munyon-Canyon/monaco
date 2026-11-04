import Foundation
import MonacoAPI

public enum ChatAttachmentDraft: Equatable, Sendable {
    case gif(Components.Schemas.GifInput)
    case stock(assetID: String)
    case thesisQuote(proposalID: String, start: Int?, end: Int?)

    public init?(_ input: Components.Schemas.CommentAttachmentInput, proposalID: String?) {
        switch input {
        case .gif(let gif):
            self = .gif(gif.gif)
        case .stock(let stock):
            self = .stock(assetID: stock.assetId)
        case .thesisQuote(let quote):
            guard let proposalID else { return nil }
            self = .thesisQuote(proposalID: proposalID, start: quote.start, end: quote.end)
        case .poll:
            return nil
        }
    }

    public func request() -> Components.Schemas.ChatAttachmentInput {
        switch self {
        case .gif(let gif):
            .gif(.init(kind: .gif, gif: gif))
        case .stock(let assetID):
            .stock(.init(kind: .stock, assetId: assetID))
        case .thesisQuote(let proposalID, let start, let end):
            .thesisQuote(.init(kind: .thesisQuote, proposalId: proposalID, start: start, end: end))
        }
    }

    public var fallbackBody: String {
        switch self {
        case .gif: GroupChatCopy.gifBody
        case .stock: GroupChatCopy.stockBody
        case .thesisQuote: GroupChatCopy.thesisBody
        }
    }

    func rewriting(_ error: APIError) -> APIError {
        guard case .thesisQuote = self, case .problem(let problem) = error, problem.code == .known(.invalidInput)
        else { return error }
        return .problem(
            ProblemError(
                status: problem.status, code: problem.code, message: GroupChatCopy.noThesis,
                traceID: problem.traceID, retryable: problem.retryable))
    }
}

extension Components.Schemas.CommentAttachment {
    var fallbackBody: String? {
        switch self {
        case .gif: GroupChatCopy.gifBody
        case .stock: GroupChatCopy.stockBody
        case .thesisQuote: GroupChatCopy.thesisBody
        case .poll: nil
        }
    }
}
