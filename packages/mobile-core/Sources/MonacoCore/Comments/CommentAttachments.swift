import Foundation
import MonacoAPI

public enum CommentAttachmentDraft: Equatable, Sendable {
    case thesis(start: Int, end: Int)
    case stock(assetID: String)

    public var kindName: String {
        switch self {
        case .thesis: "quote"
        case .stock: "stock"
        }
    }

    public var input: Components.Schemas.CommentAttachmentInput {
        switch self {
        case .thesis(let start, let end): .thesisQuote(.init(kind: .thesisQuote, start: start, end: end))
        case .stock(let assetID): .stock(.init(kind: .stock, assetId: assetID))
        }
    }
}

public enum ThesisQuote {
    public static func runeCount(_ text: String) -> Int { text.unicodeScalars.count }

    public static func runeRange(of selection: Range<String.Index>, in text: String) -> (start: Int, end: Int) {
        (
            start: text.unicodeScalars.distance(from: text.unicodeScalars.startIndex, to: selection.lowerBound),
            end: text.unicodeScalars.distance(from: text.unicodeScalars.startIndex, to: selection.upperBound)
        )
    }

    public static func all(_ text: String) -> CommentAttachmentDraft { .thesis(start: 0, end: runeCount(text)) }

    public static func selection(_ range: Range<String.Index>, in text: String) -> CommentAttachmentDraft {
        let runes = runeRange(of: range, in: text)
        return .thesis(start: runes.start, end: runes.end)
    }

    public static func isQuotable(_ thesis: String?) -> Bool {
        guard let thesis else { return false }
        return !thesis.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
}

extension CommentsCopy {
    public static func replacedAttachment(_ kind: String) -> String {
        "Replaced the \(kind)"
    }
}
