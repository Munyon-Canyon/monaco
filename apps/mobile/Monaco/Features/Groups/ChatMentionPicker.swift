import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatMentionPicker: View {
    let matches: [Components.Schemas.CabalMember]
    let pick: (Components.Schemas.CabalMember) -> Void

    var body: some View {
        VStack(spacing: 0) {
            ForEach(matches, id: \.userId) { member in
                Button {
                    pick(member)
                } label: {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        MonacoAvatar(
                            photoURL: member.photoUrl, displayName: member.displayName, size: 32, seed: member.userId)
                        VStack(alignment: .leading, spacing: 0) {
                            Text(member.displayName)
                                .font(MonacoTheme.Typo.bodyStrong)
                                .foregroundStyle(MonacoTheme.ink)
                                .lineLimit(1)
                            Text("@\(member.handle ?? "")")
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .lineLimit(1)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("chat-mention-\(member.handle ?? "")")
            }
        }
        .background(MonacoTheme.background)
        .overlay(alignment: .top) { MonacoRule() }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-mention-picker")
    }
}
