import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatScreen: View {
    let cabalID: String
    let session: ChatSession?
    let cabal: Components.Schemas.Cabal?
    let openProfile: (String) -> Void

    @State private var chat: ChatSession.State?
    @State private var toast: MonacoToast?
    @FocusState private var composerFocused: Bool
    @Environment(\.scenePhase) private var scenePhase

    private var sessionID: ObjectIdentifier? { session.map(ObjectIdentifier.init) }
    private var title: String { GroupChatCopy.title(groupName: cabal?.name) }

    var body: some View {
        VStack(spacing: 0) {
            messagesArea
            bottomBar
        }
        .background(MonacoTheme.background)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) { titleView }
        }
        .task(id: sessionID) { await observe() }
        .task(id: sessionID) { await session?.open() }
        .onChange(of: scenePhase) { _, phase in handle(phase) }
        .onChange(of: chat?.notice) { _, notice in show(notice) }
        .onDisappear { Task { await session?.close() } }
        .monacoToast($toast)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-view")
    }

    private var titleView: some View {
        HStack(spacing: 8) {
            CabalMark(groupId: cabalID, name: title, size: 28, pictureUrl: cabal?.pictureUrl)
                .accessibilityHidden(true)
            Text(title)
                .font(MonacoTheme.Typo.bodyStrong)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
        }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("chat-title")
    }

    @ViewBuilder private var messagesArea: some View {
        if let chat, chat.timeline.hasLoadedNewest {
            if chat.timeline.rows.isEmpty {
                GroupChatEmptyView(cabalID: cabalID, title: title, pictureUrl: cabal?.pictureUrl)
                    .contentShape(Rectangle())
                    .onTapGesture { composerFocused = true }
            } else {
                thread(chat)
            }
        } else if case .failed = chat?.load {
            GroupChatLoadFailureView { Task { await session?.reload() } }
        } else {
            ChatSkeleton()
        }
    }

    private func thread(_ chat: ChatSession.State) -> some View {
        GroupChatThreadView(
            rows: chat.timeline.rows,
            hasOlder: chat.timeline.hasOlder,
            isLoadingOlder: chat.isLoadingOlder,
            openProfile: openProfile,
            retry: { key in Task { await session?.retry(key: key) } },
            loadOlder: { Task { await session?.loadOlder() } },
            refresh: { await session?.reload() }
        )
    }

    @ViewBuilder private var bottomBar: some View {
        if chat?.isClosed == true {
            GroupChatClosedNotice()
        } else {
            ChatComposerBar(focus: $composerFocused) { body in
                Task { await session?.send(body: body) }
            }
        }
    }

    private func observe() async {
        guard let session else { return }
        for await state in await session.states() {
            chat = state
        }
    }

    private func handle(_ phase: ScenePhase) {
        guard let session else { return }
        switch phase {
        case .background: Task { await session.close() }
        case .active: Task { await session.open() }
        default: return
        }
    }

    private func show(_ notice: ChatSession.Notice?) {
        guard let notice else { return }
        toast = MonacoToast(message: ToastCopy.message(for: notice.error))
    }
}
