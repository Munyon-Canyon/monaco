import MonacoAPI

public struct CommentPollDraft: Equatable, Sendable {
    public static let questionLimit = 100
    public static let optionLimit = 40
    public static let minOptions = 2
    public static let maxOptions = 4
    public static let closeChoices: [CloseChoice] = [.hour, .day, .threeDays, .week]

    public enum CloseChoice: Int, CaseIterable, Sendable {
        case hour = 1
        case day = 24
        case threeDays = 72
        case week = 168

        public var label: String {
            switch self {
            case .hour: "1h"
            case .day: "24h"
            case .threeDays: "3d"
            case .week: "7d"
            }
        }
    }

    public var question: String
    public var options: [String]
    public var closes: CloseChoice

    public init(question: String = "", options: [String] = ["", ""], closes: CloseChoice = .day) {
        self.question = question
        self.options = options
        self.closes = closes
    }

    public var canAddOption: Bool { options.count < Self.maxOptions }

    public mutating func addOption() {
        guard canAddOption else { return }
        options.append("")
    }

    public var input: Components.Schemas.CommentAttachmentInput? {
        let question = question.trimmingCharacters(in: .whitespacesAndNewlines)
        let labels = options.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        guard (1...Self.questionLimit).contains(question.unicodeScalars.count),
            (Self.minOptions...Self.maxOptions).contains(labels.count),
            labels.allSatisfy({ (1...Self.optionLimit).contains($0.unicodeScalars.count) }),
            Set(labels.map { $0.lowercased() }).count == labels.count
        else { return nil }
        return .poll(.init(kind: .poll, question: question, options: labels, closesInHours: closes.rawValue))
    }

    public var isValid: Bool { input != nil }
}

extension CommentsCopy {
    public static func pollTitle(isCabal: Bool) -> String { isCabal ? "Ask the cabal" : "Ask a question" }
}

extension CommentsCopy {
    public static let pollMenu = "Poll"
}
