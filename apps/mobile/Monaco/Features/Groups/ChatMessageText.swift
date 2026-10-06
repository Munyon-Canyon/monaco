import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatMention {
    static let scheme = "monaco-mention"

    static func userID(from url: URL) -> String? {
        guard url.scheme == scheme else { return nil }
        return url.host(percentEncoded: false)
    }

    static func attributed(
        body: String, members: [Components.Schemas.CabalMember], color: Color
    ) -> AttributedString {
        var text = AttributedString(body)
        text.font = MonacoTheme.Typo.body
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
    let openProfile: (String) -> Void

    var body: some View {
        Text(ChatMention.attributed(body: text, members: members, color: color))
            .foregroundStyle(color)
            .tint(color)
            .environment(
                \.openURL,
                OpenURLAction { url in
                    guard let userID = ChatMention.userID(from: url) else { return .discarded }
                    openProfile(userID)
                    return .handled
                })
    }
}
