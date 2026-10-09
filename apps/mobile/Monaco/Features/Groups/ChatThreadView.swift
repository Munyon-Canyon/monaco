import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatThreadView: View {
    let cabalID: String
    let parentID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var thread: ThreadSession?
    @State private var chat: ChatSession?
    @State private var channelParent: ChatMessage?
    @State private var cabal: CabalModel?

    var body: some View {
        ChatThreadScreen(
            thread: thread,
            loadingParent: channelParent,
            members: cabal?.cabal?.members ?? [],
            viewerID: environment.viewer?.userID ?? "",
            deleteMessage: { id in await chat?.delete(messageId: id) },
            openProfile: { userID in
                environment.navigator.open(UserProfileRoute(userID: userID), in: environment.navigator.selectedTab)
            }
        )
        .task {
            let model = preparedCabal()
            _ = await preparedThread()
            await model.load()
        }
    }

    private func preparedCabal() -> CabalModel {
        if let cabal { return cabal }
        let created = CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        cabal = created
        return created
    }

    private func preparedThread() async -> ThreadSession {
        if let thread { return thread }
        let chat = ChatSessionRegistry.session(for: cabalID) ?? standaloneChat()
        self.chat = chat
        for await state in await chat.states() {
            channelParent = state.timeline.message(id: parentID)
            break
        }
        let created = await chat.thread(parentId: parentID)
        thread = created
        return created
    }

    private func standaloneChat() -> ChatSession {
        let created = ChatSession(
            cabalID: cabalID,
            viewerID: environment.viewer?.userID ?? "",
            api: environment.api,
            realtime: environment.chatRealtime,
            now: { Date() }
        )
        ChatSessionRegistry.register(created)
        return created
    }
}

struct ChatThreadScreen: View {
    let thread: ThreadSession?
    var loadingParent: ChatMessage?
    var members: [Components.Schemas.CabalMember] = []
    var viewerID = ""
    let deleteMessage: (String) async -> APIError?
    let openProfile: (String) -> Void

    @State private var state: ThreadSession.State?
    @State private var messageToDelete: String?
    @State private var alsoInChannel = false
    @Environment(ToastCenter.self) private var toasts
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
        .toolbar(.hidden, for: .tabBar)
        .task(id: sessionID) { await observe() }
        .task(id: sessionID) { await thread?.open() }
        .onChange(of: scenePhase) { _, phase in handle(phase) }
        .onChange(of: state?.notice) { _, notice in show(notice) }
        .onDisappear { Task { await thread?.close() } }
        .chatDeleteConfirmation(messageID: $messageToDelete) { id in delete(id) }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-thread-screen")
    }

    @ViewBuilder private var content: some View {
        if let state, let parent = state.parent {
            ChatThreadList(
                parent: parent,
                members: members,
                rows: state.timeline.rows,
                hasOlder: state.timeline.hasOlder,
                isLoadingOlder: state.isLoadingOlder,
                openProfile: openProfile,
                replyHere: { composerFocused = true },
                requestDelete: { messageToDelete = $0 },
                retry: { key in Task { await thread?.retry(key: key) } },
                discard: { key in Task { await thread?.discard(key: key) } },
                loadOlder: { Task { await thread?.loadOlder() } },
                refresh: { await thread?.reload() }
            )
        } else if case .failed = state?.load {
            GroupChatLoadFailureView(message: ChatThreadCopy.loadFailure) { Task { await thread?.reload() } }
        } else if state?.isClosed == true {
            EmptyState(title: GroupChatCopy.closed, isOnlyContent: true)
        } else if let loadingParent {
            VStack(alignment: .leading, spacing: 0) {
                ChatThreadParent(parent: loadingParent, members: members, openProfile: openProfile)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                ChatSkeleton(bottomAligned: false)
            }
        } else {
            ChatSkeleton()
        }
    }

    @ViewBuilder private var bottomBar: some View {
        if state?.isClosed == true {
            if state?.parent != nil { GroupChatClosedNotice() }
        } else if let parent = state?.parent, !parent.deleted {
            VStack(spacing: 0) {
                Toggle(ChatThreadCopy.alsoInChannel, isOn: $alsoInChannel)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .tint(MonacoTheme.accent)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.s)
                    .accessibilityIdentifier("chat-thread-also-in-channel")
                ChatComposerBar(
                    focus: $composerFocused, placeholder: ChatThreadCopy.composerPlaceholder,
                    members: members, viewerID: viewerID
                ) { body in
                    let alsoInChannel = alsoInChannel
                    self.alsoInChannel = false
                    Task { await thread?.send(body: body, alsoInChannel: alsoInChannel) }
                }
            }
        } else if state?.parent != nil {
            Text(ChatThreadCopy.parentDeleted)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityIdentifier("chat-thread-parent-deleted")
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
            toasts.current = MonacoToast(message: failure.map(ToastCopy.message(for:)) ?? GroupChatCopy.deleted)
        }
    }

    private func show(_ notice: ChatSession.Notice?) {
        guard let notice else { return }
        toasts.current = MonacoToast(message: ToastCopy.message(for: notice.error))
    }
}

struct ChatThreadList: View {
    let parent: ChatMessage
    let members: [Components.Schemas.CabalMember]
    let rows: [ChatRow]
    let hasOlder: Bool
    let isLoadingOlder: Bool
    let openProfile: (String) -> Void
    let replyHere: () -> Void
    let requestDelete: (String) -> Void
    let retry: (String) -> Void
    let discard: (String) -> Void
    let loadOlder: () -> Void
    let refresh: () async -> Void

    @State private var now = Date()

    var body: some View {
        ChatList(
            rows: rows, hasOlder: hasOlder, isLoadingOlder: isLoadingOlder,
            loadEarlierID: "chat-thread-load-earlier", listID: "chat-thread-list",
            loadOlder: loadOlder, refresh: refresh
        ) {
            ChatThreadParent(parent: parent, members: members, openProfile: openProfile)
        } content: {
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
                    row: row, now: now, members: members, openProfile: openProfile, retry: retry,
                    discard: discard, replyHere: replyHere, requestDelete: requestDelete
                )
                .id(row.id)
            }
        }
        .onChange(of: rows) { now = Date() }
    }
}

struct ChatThreadParent: View {
    let parent: ChatMessage
    let members: [Components.Schemas.CabalMember]
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
            parentBody
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("chat-thread-parent-body")
            MonacoRule().padding(.top, MonacoTheme.Space.s)
        }
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.s)
    }

    @ViewBuilder private var parentBody: some View {
        if parent.deleted {
            Text(GroupChatCopy.deleted)
                .font(MonacoTheme.Typo.body)
                .italic()
                .foregroundStyle(MonacoTheme.muted)
        } else {
            ChatMessageText(
                text: parent.body ?? "", members: members, color: MonacoTheme.ink,
                mentionColor: MonacoTheme.brand, openProfile: openProfile)
            Text(parent.createdAt, format: .dateTime.hour().minute())
                .font(MonacoTheme.Typo.stamp)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .accessibilityIdentifier("chat-thread-parent-time")
        }
    }
}
