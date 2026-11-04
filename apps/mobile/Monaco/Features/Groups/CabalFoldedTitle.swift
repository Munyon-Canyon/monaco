import MonacoCore
import SwiftUI

struct CabalFoldedTitle: View {
    let groupId: String
    let name: String
    let pictureUrl: String?
    let memberCount: Int32
    let alwaysShown: Bool
    let scrolledShown: Bool

    private var shown: Bool { alwaysShown || scrolledShown }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            CabalMark(groupId: groupId, name: name, size: .small, pictureUrl: pictureUrl)
                .fixedSize()
                .layoutPriority(1)
            VStack(alignment: .leading, spacing: 0) {
                Text(CabalHeaderCopy.shortName(name))
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                    .truncationMode(.tail)
                Text(CabalCopy.memberCount(memberCount))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            }
        }
        .opacity(shown ? 1 : 0)
        .animation(.easeOut(duration: 0.15), value: shown)
        .accessibilityHidden(!shown)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.accessibilityLabel(name: name, memberCount: memberCount))
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("cabal-inline-title")
    }

    static func accessibilityLabel(name: String, memberCount: Int32) -> String {
        "\(CabalHeaderCopy.shortName(name)), \(CabalCopy.memberCount(memberCount))"
    }
}
