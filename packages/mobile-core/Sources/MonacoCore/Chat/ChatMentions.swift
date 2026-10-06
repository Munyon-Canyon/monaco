import Foundation
import MonacoAPI

private func isHandleCharacter(_ c: Character) -> Bool {
    c.isASCII && (c.isLetter || c.isNumber || c == "_")
}

public enum MentionQuery {
    public static let maxLength = 20
    public static let maxMatches = 5

    public static func active(in text: String, cursor: Int) -> String? {
        guard let start = atOffset(in: text, cursor: cursor) else { return nil }
        let chars = Array(text)
        return String(chars[(start + 1)..<cursor])
    }

    static func atOffset(in text: String, cursor: Int) -> Int? {
        let chars = Array(text)
        guard cursor >= 0, cursor <= chars.count else { return nil }
        var i = cursor
        while i > 0, isHandleCharacter(chars[i - 1]) { i -= 1 }
        guard i > 0, chars[i - 1] == "@", cursor - i <= maxLength else { return nil }
        let at = i - 1
        if at > 0 {
            let before = chars[at - 1]
            if isHandleCharacter(before) || before == "@" { return nil }
        }
        return at
    }

    public static func matches(
        _ query: String, members: [Components.Schemas.CabalMember], viewerID: String
    ) -> [Components.Schemas.CabalMember] {
        let prefix = query.lowercased()
        let candidates: [(handle: String, member: Components.Schemas.CabalMember)] = members.compactMap { member in
            guard member.userId != viewerID, let handle = member.handle else { return nil }
            let lowered = handle.lowercased()
            return lowered.hasPrefix(prefix) ? (lowered, member) : nil
        }
        return candidates.sorted { $0.handle < $1.handle }.prefix(maxMatches).map(\.member)
    }
}

public enum MentionInsertion {
    public static func insert(handle: String, into text: String, cursor: Int) -> (text: String, cursor: Int) {
        guard let at = MentionQuery.atOffset(in: text, cursor: cursor) else { return (text, cursor) }
        let chars = Array(text)
        let inserted = "@\(handle) "
        let result = String(chars[..<at]) + inserted + String(chars[cursor...])
        return (result, at + inserted.count)
    }
}

public enum MentionRanges {
    private static var pattern: Regex<(Substring, handle: Substring)> {
        #/(?:^|[^A-Za-z0-9_@])@(?<handle>[A-Za-z0-9_]{3,20})/#
    }

    public static func ranges(
        in body: String, members: [Components.Schemas.CabalMember]
    ) -> [(range: Range<String.Index>, userID: String)] {
        var byHandle: [String: String] = [:]
        for member in members {
            if let handle = member.handle { byHandle[handle.lowercased()] = member.userId }
        }
        return body.matches(of: pattern).compactMap { match in
            let handle = match.output.handle
            guard let userID = byHandle[handle.lowercased()],
                let start = body.index(handle.startIndex, offsetBy: -1, limitedBy: body.startIndex)
            else { return nil }
            return (start..<handle.endIndex, userID)
        }
    }
}
