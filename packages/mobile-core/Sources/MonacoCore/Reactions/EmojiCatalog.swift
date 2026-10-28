import Foundation

public enum EmojiGroup: String, Codable, CaseIterable, Sendable {
    case smileys, people, nature, food, activities, travel, objects, symbols, flags
}

public struct EmojiEntry: Equatable, Sendable {
    public init(emoji: String, name: String, keywords: [String], group: EmojiGroup) {
        self.emoji = emoji
        self.name = name
        self.keywords = keywords
        self.group = group
    }

    public let emoji: String
    public let name: String
    public let keywords: [String]
    public let group: EmojiGroup
}

extension EmojiEntry: Decodable {
    private enum CodingKeys: String, CodingKey {
        case emoji = "c"
        case name = "n"
        case keywords = "k"
        case group = "g"
    }
}

public struct EmojiCatalog: Sendable {
    public let entries: [EmojiEntry]

    private let names: [String: String]
    private let words: [[String]]

    public static func load(from data: Data) throws -> EmojiCatalog {
        EmojiCatalog(entries: try JSONDecoder().decode([EmojiEntry].self, from: data))
    }

    public init(entries: [EmojiEntry]) {
        self.entries = entries
        names = Dictionary(entries.map { ($0.emoji, $0.name) }, uniquingKeysWith: { first, _ in first })
        words = entries.map { entry in
            ([entry.name] + entry.keywords).flatMap { Self.tokens($0) }
        }
    }

    public func entries(in group: EmojiGroup) -> [EmojiEntry] {
        entries.filter { $0.group == group }
    }

    public func name(for emoji: String) -> String? {
        names[emoji] ?? names[emoji.replacingOccurrences(of: "\u{FE0F}", with: "")]
            ?? names[emoji + "\u{FE0F}"]
    }

    public func search(_ query: String) -> [EmojiEntry] {
        let wanted = Self.tokens(query)
        guard !wanted.isEmpty else { return [] }
        let folded = Self.fold(query)
        var exact: [EmojiEntry] = []
        var leading: [EmojiEntry] = []
        var rest: [EmojiEntry] = []
        for (index, entry) in entries.enumerated() {
            guard wanted.allSatisfy({ token in words[index].contains { $0.hasPrefix(token) } }) else { continue }
            let name = Self.fold(entry.name)
            if name == folded {
                exact.append(entry)
            } else if name.hasPrefix(folded) {
                leading.append(entry)
            } else {
                rest.append(entry)
            }
        }
        return exact + leading + rest
    }

    private static func fold(_ text: String) -> String {
        text.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: nil)
            .trimmingCharacters(in: .whitespaces)
    }

    private static func tokens(_ text: String) -> [String] {
        fold(text).split { !$0.isLetter && !$0.isNumber && $0 != "+" }.map(String.init)
    }
}
