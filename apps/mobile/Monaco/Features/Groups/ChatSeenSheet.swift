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
                    ForEach(Array(members.enumerated()), id: \.element.userId) { index, member in
                        row(member, isLast: index == members.count - 1)
                    }
                }
            }
        }
    }

    private func row(_ member: ChatSeenModel.Member, isLast: Bool) -> some View {
        Button {
            dismiss()
            openProfile(member.userId)
        } label: {
            MonacoRow(title: ChatSeenCopy.name(member), subtitle: ChatSeenCopy.handle(member), isLast: isLast) {
                MonacoAvatar(photoURL: member.photoUrl, displayName: member.displayName, seed: member.userId)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("chat-seen-row-\(member.userId)")
    }

    private func preparedModel() -> ChatSeenModel {
        if let model { return model }
        let created = ChatSeenModel(cabalID: cabalID, messageID: messageID, api: environment.api)
        model = created
        return created
    }
}
