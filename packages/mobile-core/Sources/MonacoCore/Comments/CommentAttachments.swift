import MonacoAPI

public enum CommentAttachmentDraft: Equatable, Sendable {
    case sample(title: String)

    public var kindName: String {
        switch self {
        case .sample: "sample"
        }
    }

    public var input: Components.Schemas.CommentAttachmentInput {
        switch self {
        case .sample: .thesisQuote(.init(kind: .thesisQuote, start: 0, end: 25))
        }
    }
}

extension CommentsCopy {
    public static func replacedAttachment(_ kind: String) -> String {
        "Replaced the \(kind)"
    }
}
