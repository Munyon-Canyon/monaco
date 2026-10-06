import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatThreadView: View {
    let cabalID: String
    let parentID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var thread: ThreadSession?
    @State private var chat: ChatSession?

    var body: some View {
        ChatThreadScreen(
            thread: thread,
            deleteMessage: { id in await chat?.delete(messageId: id) },
            openProfile: { userID in
                environment.navigator.open(UserProfileRoute(userID: userID), in: environment.navigator.selectedTab)
            }
        )
        .task { _ = await preparedThread() }
    }

    private func preparedThread() async -> ThreadSession {
        if let thread { return thread }
        let chat = ChatSessionRegistry.session(for: cabalID) ?? standaloneChat()
        self.chat = chat
        let created = await chat.thread(parentId: parentID)
        thread = created
        return created
    }

    private func standaloneChat() -> ChatSession {
        let created = ChatSession(
            cabalID: cabalID,
            viewerID: environment.viewer?.userID ?? "",
            api: environment.api,
            realtime: AblyChatRealtime(api: environment.api),
            now: { Date() }
        )
        ChatSessionRegistry.register(created)
        return created
    }
}

struct ChatThreadScreen: View {
    let thread: ThreadSession?
    let deleteMessage: (String) async -> APIError?
    let openProfile: (String) -> Void

    @State private var state: ThreadSession.State?
    @State private var messageToDelete: String?
    @State private var alsoInChannel = false
    @State private var toast: MonacoToast?
    @FocusState private var composerFocused: Bool
    @Environment(\.scenePhase) private var scenePhase

    private var sessionID: ObjectIdentifier? { thread.map(ObjectIdentifier.init) }

    var body: some View {
        VStack(spacing: 0) {
            content
            bottomBar
        }
        .background(MonacoTheme.background)
        .navigationTitle(ChatThreadCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .task(id: sessionID) { await observe() }
        .task(id: sessionID) { await thread?.open() }
        .onChange(of: scenePhase) { _, phase in handle(phase) }
        .onChange(of: state?.notice) { _, notice in show(notice) }
        .onDisappear { Task { await thread?.close() } }
        .chatDeleteConfirmation(messageID: $messageToDelete) { id in delete(id) }
        .monacoToast($toast)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-thread-screen")
    }

    @ViewBuilder private var content: some View {
        if let state, let parent = state.parent {
            ChatThreadList(
                parent: parent,
                rows: state.timeline.rows,
                hasOlder: state.timeline.hasOlder,
                isLoadingOlder: state.isLoadingOlder,
                openProfile: openProfile,
                replyHere: { composerFocused = true },
                requestDelete: { messageToDelete = $0 },
                retry: { key in Task { await thread?.retry(key: key) } },
                loadOlder: { Task { await thread?.loadOlder() } },
                refresh: { await thread?.reload() }
            )
        } else if case .failed = state?.load {
            GroupChatLoadFailureView(message: ChatThreadCopy.loadFailure) { Task { await thread?.reload() } }
        } else if state?.isClosed == true {
            Spacer()
        } else {
            ChatSkeleton()
        }
    }

    @ViewBuilder private var bottomBar: some View {
        if state?.isClosed == true {
            GroupChatClosedNotice()
        } else if state?.parent != nil {
            VStack(spacing: 0) {
                Toggle(ChatThreadCopy.alsoInChannel, isOn: $alsoInChannel)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .tint(MonacoTheme.accent)
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .padding(.vertical, MonacoTheme.Space.s)
                    .accessibilityIdentifier("chat-thread-also-in-channel")
                ChatComposerBar(focus: $composerFocused, placeholder: ChatThreadCopy.composerPlaceholder) { body in
                    let alsoInChannel = alsoInChannel
                    self.alsoInChannel = false
                    Task { await thread?.send(body: body, alsoInChannel: alsoInChannel) }
                }
            }
        }
    }

    private func observe() async {
        guard let thread else { return }
        for await next in await thread.states() {
            state = next
        }
    }

    private func handle(_ phase: ScenePhase) {
        guard let thread else { return }
        switch phase {
        case .background: Task { await thread.close() }
        case .active: Task { await thread.open() }
        default: return
        }
    }

    private func delete(_ id: String) {
        Task {
            let failure = await deleteMessage(id)
            toast = MonacoToast(message: failure.map(ToastCopy.message(for:)) ?? GroupChatCopy.deleted)
        }
    }

    private func show(_ notice: ChatSession.Notice?) {
        guard let notice else { return }
        toast = MonacoToast(message: ToastCopy.message(for: notice.error))
    }
}

struct ChatThreadList: View {
    let parent: ChatMessage
    let rows: [ChatRow]
    let hasOlder: Bool
    let isLoadingOlder: Bool
    let openProfile: (String) -> Void
    let replyHere: () -> Void
    let requestDelete: (String) -> Void
    let retry: (String) -> Void
    let loadOlder: () -> Void
    let refresh: () async -> Void

    private static let bottomAnchor = "thread-bottom"

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 3) {
                    ChatThreadParent(parent: parent, openProfile: openProfile)
                    if hasOlder { loadEarlierButton }
                    if rows.isEmpty {
                        Text(ChatThreadCopy.noReplies)
                            .font(MonacoTheme.Typo.callout)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 24)
                            .accessibilityIdentifier("chat-thread-empty")
                    }
                    ForEach(rows) { row in
                        GroupChatRowView(
                            row: row, now: Date(), openProfile: openProfile, retry: retry,
                            replyHere: replyHere, requestDelete: requestDelete
                        )
                        .id(row.id)
                    }
                    Color.clear.frame(height: 1).id(Self.bottomAnchor)
                }
                .padding(.horizontal, 16)
                .padding(.bottom, 12)
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await refresh() }
            .onChange(of: rows.last?.id) { _, _ in
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomAnchor, anchor: .bottom) }
            }
            .accessibilityIdentifier("chat-thread-list")
        }
    }

    private var loadEarlierButton: some View {
        Button {
            loadOlder()
        } label: {
            if isLoadingOlder {
                ProgressView().tint(MonacoTheme.accent)
            } else {
                Text(GroupChatCopy.loadEarlier).font(MonacoTheme.Typo.captionStrong)
            }
        }
        .buttonStyle(.borderless)
        .foregroundStyle(MonacoTheme.accent)
        .frame(maxWidth: .infinity)
        .padding(.vertical, 8)
        .disabled(isLoadingOlder)
        .accessibilityIdentifier("chat-thread-load-earlier")
    }
}

struct ChatThreadParent: View {
    let parent: ChatMessage
    let openProfile: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Button {
                openProfile(parent.author.id)
            } label: {
                HStack(spacing: MonacoTheme.Space.s) {
                    MonacoAvatar(
                        photoURL: parent.author.photoUrl, displayName: parent.author.displayName, size: 32,
                        seed: parent.author.id)
                    Text(parent.author.displayName)
                        .font(MonacoTheme.Typo.bodyStrong)
                        .foregroundStyle(MonacoTheme.ink)
                        .lineLimit(1)
                }
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(parent.author.displayName.isEmpty)
            .accessibilityIdentifier("chat-thread-parent-author")
            Text(parent.deleted ? GroupChatCopy.deleted : (parent.body ?? ""))
                .font(MonacoTheme.Typo.body)
                .italic(parent.deleted)
                .foregroundStyle(parent.deleted ? MonacoTheme.muted : MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("chat-thread-parent-body")
            MonacoRule().padding(.top, MonacoTheme.Space.s)
        }
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.s)
    }
}
