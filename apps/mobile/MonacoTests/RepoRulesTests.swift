import Foundation
import Testing

struct RepoRulesTests {
    static let taskOnEmptyViewOwners: [String: Int] = [
        "Features/Proposals/ProposeBuyStockView.swift": 612
    ]

    @Test
    func noLoadingModifierSitsOnAViewThatCanRenderNothing() throws {
        let flagged = try Self.appSources().flatMap { path, source in
            TaskOnEmptyViewRule.violations(in: source).map { (path: path, line: $0) }
        }
        let unowned = flagged.filter { Self.taskOnEmptyViewOwners[$0.path] == nil }.map { "\($0.path):\($0.line)" }
        #expect(unowned.isEmpty, "a .task or .onAppear never runs on a view with no content: \(unowned)")

        let stale = Set(Self.taskOnEmptyViewOwners.keys).subtracting(flagged.map(\.path))
        #expect(stale.isEmpty, "owned files no longer flagged, drop them from the allow list: \(stale.sorted())")
    }

    private static func appSources(filePath: String = #filePath) throws -> [(String, String)] {
        let root = URL(filePath: filePath).deletingLastPathComponent().deletingLastPathComponent()
            .appending(path: "Monaco")
        let files = try #require(FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil))
        return try files.compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }.map { url in
            let path = String(url.standardizedFileURL.path.dropFirst(root.standardizedFileURL.path.count + 1))
            return (path, try String(contentsOf: url, encoding: .utf8))
        }
    }
}

enum TaskOnEmptyViewRule {
    static let loadingModifiers: Set<String> = ["task", "onAppear"]

    static func violations(in source: String) -> [Int] {
        let text = Array(source)
        let receivers = groups(in: text) + emptyViews(in: text) + computedViews(in: text, source: source)
        return Set(receivers.filter { loads(after: $0.end, in: text) }.map { line(of: $0.start, in: text) }).sorted()
    }

    private struct Receiver {
        let start: Int
        let end: Int
    }

    private static func groups(in text: [Character]) -> [Receiver] {
        matches(of: #"\bGroup\s*\{"#, in: String(text)).compactMap { range in
            let open = range.upperBound - 1
            guard let close = closing(open, in: text),
                canRenderNothing(String(text[(open + 1)..<close]))
            else { return nil }
            return Receiver(start: range.lowerBound, end: close + 1)
        }
    }

    private static func emptyViews(in text: [Character]) -> [Receiver] {
        matches(of: #"\bEmptyView\(\)"#, in: String(text)).map { Receiver(start: $0.lowerBound, end: $0.upperBound) }
    }

    private static func computedViews(in text: [Character], source: String) -> [Receiver] {
        let declarations = matches(of: #"\bvar (\w+): some View \{"#, in: source)
        return declarations.flatMap { range -> [Receiver] in
            let name = String(String(text[range]).dropFirst(4).prefix { $0 != ":" })
            let open = range.upperBound - 1
            guard name != "body", let close = closing(open, in: text),
                canRenderNothing(String(text[(open + 1)..<close]))
            else { return [] }
            return matches(of: #"(?m)^[ \t]*\#(name)(?=\s*\.)"#, in: source).map { use in
                Receiver(start: use.upperBound - name.count, end: use.upperBound)
            }
        }
    }

    private static func canRenderNothing(_ body: String) -> Bool {
        let text = Array(body.trimmingCharacters(in: .whitespacesAndNewlines))
        if text.starts(with: Array("if ")) {
            return ifWithoutElse(text)
        }
        if text.starts(with: Array("switch ")),
            let open = text.firstIndex(of: "{"),
            let close = closing(open, in: text),
            text[(close + 1)...].allSatisfy(\.isWhitespace)
        {
            let cases = String(text[(open + 1)..<close])
            return !matches(of: #":\s*EmptyView\(\)\s*(?=case\s|default\s*:|$)"#, in: cases).isEmpty
        }
        return false
    }

    private static func ifWithoutElse(_ text: [Character]) -> Bool {
        var index = 0
        while let open = text[index...].firstIndex(of: "{"), let close = closing(open, in: text) {
            index = skipWhitespace(from: close + 1, in: text)
            guard text[index...].starts(with: Array("else")) else {
                return index == text.count
            }
            index = skipWhitespace(from: index + 4, in: text)
            if index < text.count, text[index] == "{" {
                return false
            }
        }
        return false
    }

    private static func loads(after end: Int, in text: [Character]) -> Bool {
        var index = skipWhitespace(from: end, in: text)
        while index < text.count, text[index] == "." {
            let name = String(text[(index + 1)...].prefix { $0.isLetter || $0.isNumber || $0 == "_" })
            if loadingModifiers.contains(name) {
                return true
            }
            index += 1 + name.count
            for bracket: (Character, Character) in [("(", ")"), ("{", "}")] {
                while index < text.count, text[index] == " " { index += 1 }
                if index < text.count, text[index] == bracket.0 {
                    guard let close = closing(index, in: text, open: bracket.0, close: bracket.1) else { return false }
                    index = close + 1
                }
            }
            index = skipWhitespace(from: index, in: text)
        }
        return false
    }

    private static func closing(
        _ open: Int, in text: [Character], open opener: Character = "{", close closer: Character = "}"
    ) -> Int? {
        var depth = 0
        var inString = false
        var index = open
        while index < text.count {
            let character = text[index]
            if character == "\\" && inString {
                index += 2
                continue
            }
            if character == "\"" {
                inString.toggle()
            } else if !inString && character == opener {
                depth += 1
            } else if !inString && character == closer {
                depth -= 1
                if depth == 0 { return index }
            }
            index += 1
        }
        return nil
    }

    private static func skipWhitespace(from start: Int, in text: [Character]) -> Int {
        var index = start
        while index < text.count, text[index].isWhitespace { index += 1 }
        return index
    }

    private static func matches(of pattern: String, in source: String) -> [Range<Int>] {
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return [] }
        let utf16 = Array(source.utf16)
        let characterOffsets = characterIndexByUTF16Offset(source)
        return regex.matches(in: source, range: NSRange(location: 0, length: utf16.count)).compactMap { match in
            guard let lower = characterOffsets[match.range.location],
                let upper = characterOffsets[match.range.location + match.range.length]
            else { return nil }
            return lower..<upper
        }
    }

    private static func characterIndexByUTF16Offset(_ source: String) -> [Int: Int] {
        var offsets: [Int: Int] = [:]
        var utf16Offset = 0
        for (characterIndex, character) in source.enumerated() {
            offsets[utf16Offset] = characterIndex
            utf16Offset += character.utf16.count
        }
        offsets[utf16Offset] = source.count
        return offsets
    }

    private static func line(of index: Int, in text: [Character]) -> Int {
        text[..<index].filter { $0 == "\n" }.count + 1
    }
}
