import MonacoCore
import SwiftUI

struct MembersReplyLine: View {
    let cabalName: String
    let pictureURL: String?

    private static let crest: CGFloat = 24

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            CabalMark(groupId: cabalName, name: cabalName, size: Self.crest, pictureUrl: pictureURL)
            Text(CommentsCopy.membersReplyHere(cabalName))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, MonacoTheme.Space.m)
        .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) { MonacoRule() }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("comment-members-only")
        .accessibilityLabel(CommentsCopy.membersReplyHere(cabalName))
        .allowsHitTesting(false)
    }
}
