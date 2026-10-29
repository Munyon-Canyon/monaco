import MonacoCore
import SwiftUI

struct FeedPostHeader: View {
    let name: String
    let verb: String
    let age: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
            Text(name)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .layoutPriority(2)
                .monacoPrivate()
            if !verb.isEmpty {
                Text(verb)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            }
            Spacer(minLength: MonacoTheme.Space.s)
            Text(age)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize()
                .layoutPriority(3)
        }
        .frame(minHeight: FeedPostMetrics.lead)
    }
}
