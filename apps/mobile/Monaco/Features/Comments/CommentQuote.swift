import MonacoCore
import SwiftUI

struct CommentQuote: View {
    let quote: CommentQuoteRow
    let rowID: String
    let onJump: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private enum Metrics {
        static let rule: CGFloat = 2
        static let tap: CGFloat = 44
    }

    var body: some View {
        if quote.canJump {
            Button(action: onJump) { line }
                .buttonStyle(.plain)
                .accessibilityIdentifier("comment-quote-\(rowID)")
        } else {
            line.accessibilityIdentifier("comment-quote-\(rowID)")
        }
    }

    private var line: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(monaco: .replyArrow)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.brand)
            (Text(quote.name).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
                + Text(" ")
                + Text(quote.excerpt).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText))
                .lineLimit(dynamicTypeSize > .large ? nil : 1)
                .truncationMode(.tail)
                .monacoPrivate()
        }
        .padding(.leading, MonacoTheme.Space.s)
        .overlay(alignment: .leading) {
            Rectangle().fill(MonacoTheme.hairline).frame(width: Metrics.rule)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, MonacoTheme.Space.sm)
        .contentShape(Rectangle())
        .padding(.vertical, -MonacoTheme.Space.sm)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Replying to \(quote.name): \(quote.excerpt)")
        .accessibilityAddTraits(quote.canJump ? .isButton : [])
    }
}
