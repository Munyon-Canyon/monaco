import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatMentionPicker: View {
    let matches: [Components.Schemas.CabalMember]
    let pick: (Components.Schemas.CabalMember) -> Void

    var body: some View {
        MonacoCard(style: .inset) {
            VStack(spacing: 0) {
                ForEach(Array(matches.enumerated()), id: \.element.userId) { index, member in
                    Button {
                        pick(member)
                    } label: {
                        MonacoRow(
                            title: member.displayName, titleFont: MonacoTheme.Typo.subheadStrong,
                            subtitle: "@\(member.handle ?? "")", isLast: index == matches.count - 1, isPrivate: true
                        ) {
                            MonacoAvatar(
                                photoURL: member.photoUrl, displayName: member.displayName, size: 32,
                                seed: member.userId)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("chat-mention-\(member.handle ?? "")")
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
        .background(MonacoTheme.background)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-mention-picker")
    }
}
