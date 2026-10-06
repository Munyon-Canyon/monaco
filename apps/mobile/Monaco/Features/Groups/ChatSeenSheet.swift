import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatSeenSheet: View {
    let cabalID: String
    let messageID: String
    let openProfile: (String) -> Void

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @State private var model: ChatSeenModel?

    var body: some View {
        NavigationStack {
            content
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .background(MonacoTheme.background)
                .navigationTitle(ChatSeenCopy.sheetTitle)
                .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium, .large])
        .task {
            let model = preparedModel()
            await model.load()
        }
        .accessibilityIdentifier("chat-seen-sheet")
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            BoardRowSkeleton(rows: 3)
                .accessibilityHidden(true)
                .accessibilityIdentifier("chat-seen-loading")
        case .failed:
            EmptyState(title: ChatSeenCopy.loadFailed, actionTitle: ChatSeenCopy.retry) {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("chat-seen-failed")
        case .loaded(let members):
            ScrollView {
                LazyVStack(spacing: 0) {
                    ForEach(members, id: \.userId) { member in
                        row(member)
                    }
                }
            }
        }
    }

    private func row(_ member: ChatSeenModel.Member) -> some View {
        Button {
            dismiss()
            openProfile(member.userId)
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                MonacoAvatar(photoURL: member.photoUrl, displayName: member.displayName, size: 40, seed: member.userId)
                VStack(alignment: .leading, spacing: 2) {
                    Text(ChatSeenCopy.name(member))
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                        .lineLimit(1)
                    if let handle = ChatSeenCopy.handle(member) {
                        Text(handle)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .lineLimit(1)
                    }
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(minHeight: 56)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("chat-seen-row-\(member.userId)")
    }

    private func preparedModel() -> ChatSeenModel {
        if let model { return model }
        let created = ChatSeenModel(cabalID: cabalID, messageID: messageID, api: environment.api)
        model = created
        return created
    }
}
