import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatMention {
    static let scheme = "monaco-mention"

    static func userID(from url: URL) -> String? {
        guard url.scheme == scheme else { return nil }
        return url.host(percentEncoded: false)
    }

    private static let linkDetector = try? NSDataDetector(types: NSTextCheckingResult.CheckingType.link.rawValue)

    static func attributed(
        body: String, members: [Components.Schemas.CabalMember], color: Color
    ) -> AttributedString {
        var text = AttributedString(body)
        text.font = MonacoTheme.Typo.body
        let whole = NSRange(body.startIndex..., in: body)
        for match in linkDetector?.matches(in: body, range: whole) ?? [] {
            guard let url = match.url, let range = Range(match.range, in: body),
                let lower = AttributedString.Index(range.lowerBound, within: text),
                let upper = AttributedString.Index(range.upperBound, within: text)
            else { continue }
            text[lower..<upper].link = url
        }
        for (range, userID) in MentionRanges.ranges(in: body, members: members) {
            guard let lower = AttributedString.Index(range.lowerBound, within: text),
                let upper = AttributedString.Index(range.upperBound, within: text)
            else { continue }
            text[lower..<upper].font = MonacoTheme.Typo.bodyStrong
            text[lower..<upper].foregroundColor = color
            text[lower..<upper].link = URL(string: "\(scheme)://\(userID)")
        }
        return text
    }
}

struct ChatMessageText: View {
    let text: String
    let members: [Components.Schemas.CabalMember]
    let color: Color
    var mentionColor: Color?
    let openProfile: (String) -> Void

    var body: some View {
        Text(ChatMention.attributed(body: text, members: members, color: mentionColor ?? color))
            .foregroundStyle(color)
            .tint(mentionColor ?? color)
            .environment(
                \.openURL,
                OpenURLAction { url in
                    guard let userID = ChatMention.userID(from: url) else { return .systemAction }
                    openProfile(userID)
                    return .handled
                })
    }
}
