import MonacoCore
import SwiftUI

struct FeedPostHeader: View {
    let name: String
    let verb: String
    let age: String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var nameText: some View {
        Text(name)
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
            .layoutPriority(2)
            .monacoPrivate()
    }

    private var verbText: some View {
        Text(verb)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.secondaryText)
    }

    private var ageText: some View {
        Text(age)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.secondaryText)
            .fixedSize()
            .layoutPriority(3)
    }

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                nameText
                if !verb.isEmpty { verbText.fixedSize(horizontal: false, vertical: true) }
                ageText
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .frame(minHeight: FeedPostMetrics.lead)
        } else {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
                nameText
                if !verb.isEmpty { verbText.lineLimit(1) }
                Spacer(minLength: MonacoTheme.Space.s)
                ageText
            }
            .frame(minHeight: FeedPostMetrics.lead)
        }
    }
}
