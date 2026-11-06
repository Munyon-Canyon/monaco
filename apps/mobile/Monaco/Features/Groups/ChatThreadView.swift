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
    @State private var proposals: ChatProposalStore?

    var body: some View {
        ChatThreadScreen(
            thread: thread,
            loadingParent: channelParent,
            cabalName: cabal?.cabal?.name ?? "",
            members: cabal?.cabal?.members ?? [],
            viewerID: environment.viewer?.userID ?? "",
            proposals: proposals,
            deleteMessage: { id in await chat?.delete(messageId: id) },
            openProfile: { userID in
                environment.navigator.open(UserProfileRoute(userID: userID), in: environment.navigator.selectedTab)
            },
            openProposal: { proposalID in
                environment.navigator.open(
                    ProposalRoute(proposalID: proposalID), in: environment.navigator.selectedTab)
            }
        )
        .task {
            let store = preparedProposals()
            let model = preparedCabal()
            _ = await preparedThread()
            await model.load()
            await store.observe(cabalID: cabalID, hints: environment.hints)
        }
    }

    private func preparedProposals() -> ChatProposalStore {
        if let proposals { return proposals }
        let created = ChatProposalStore(api: environment.api)
        proposals = created
        return created
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
            now: { Date() },
            blocked: BlockedAuthors(api: environment.api)
        )
        ChatSessionRegistry.register(created)
        return created
    }
}

enum ChatThreadMetrics {
    static let toggleRowHeight: CGFloat = 63
}

struct ChatThreadScreen: View {
    let thread: ThreadSession?
    var loadingParent: ChatMessage?
    var cabalName = ""
    var members: [Components.Schemas.CabalMember] = []
    var viewerID = ""
    var proposals: ChatProposalStore?
    let deleteMessage: (String) async -> APIError?
    let openProfile: (String) -> Void
    var openProposal: ((String) -> Void)?

    @State private var state: ThreadSession.State?
    @State private var messageToDelete: String?
    @State private var alsoInChannel = false
    @Environment(ToastCenter.self) private var toasts
    @FocusState private var composerFocused: Bool
    @Environment(\.scenePhase) private var scenePhase

    private var sessionID: ObjectIdentifier? { thread.map(ObjectIdentifier.init) }

    private var proposalID: String? { ChatThreadHead.proposalID(of: state?.parent ?? loadingParent) }

    private var proposerName: String? {
        (state?.parent ?? loadingParent)?.author.displayName
    }

    var body: some View {
        VStack(spacing: 0) {
            content
            bottomBar
        }
        .background(MonacoTheme.background)
        .modifier(ChatThreadTitle(subtitle: cabalName))
        .toolbar(.hidden, for: .tabBar)
        .toolbar {
            if let proposalID, let openProposal {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(ChatThreadCopy.proposalButton) { openProposal(proposalID) }
                        .accessibilityIdentifier("chat-thread-open-proposal")
                }
            }
        }
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
                proposals: proposals,
                openProposal: openProposal,
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
                ChatThreadHead(
                    parent: loadingParent, members: members, proposals: proposals, now: Date(),
                    openProfile: openProfile, openProposal: openProposal
                )
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
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .tint(MonacoTheme.primaryButtonFill)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .frame(minHeight: ChatThreadMetrics.toggleRowHeight)
                    .overlay(alignment: .bottom) { MonacoRule() }
                    .accessibilityIdentifier("chat-thread-also-in-channel")
                ChatComposerBar(
                    focus: $composerFocused, placeholder: composerPlaceholder,
                    members: members, viewerID: viewerID
                ) { body in
                    let alsoInChannel = alsoInChannel
                    self.alsoInChannel = false
                    Task { await thread?.send(body: body, alsoInChannel: alsoInChannel) }
                }
            }
        } else if state?.parent != nil {
            Text(ChatThreadCopy.parentDeleted)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityIdentifier("chat-thread-parent-deleted")
        }
    }

    private var composerPlaceholder: String {
        proposalID == nil
            ? ChatThreadCopy.composerPlaceholder
            : ChatThreadCopy.composerPlaceholder(proposer: proposerName)
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
    var proposals: ChatProposalStore?
    var openProposal: ((String) -> Void)?
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

    static func quotedRowID(commentID: String, in messages: [ChatMessage]) -> String? {
        messages.first { $0.commentId == commentID }?.id
    }

    var body: some View {
        ScrollViewReader { proxy in
            ChatList(
                rows: rows, hasOlder: hasOlder, isLoadingOlder: isLoadingOlder,
                loadEarlierID: "chat-thread-load-earlier", listID: "chat-thread-list",
                loadOlder: loadOlder, refresh: refresh
            ) {
                ChatThreadHead(
                    parent: parent, members: members, proposals: proposals, now: now,
                    openProfile: openProfile, openProposal: openProposal)
            } content: {
                if rows.isEmpty {
                    Text(ChatThreadCopy.noReplies)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, MonacoTheme.Space.l)
                        .accessibilityIdentifier("chat-thread-empty")
                }
                ForEach(rows) { row in
                    GroupChatRowView(
                        row: row, now: now, members: members, openProfile: openProfile, retry: retry,
                        discard: discard, replyHere: replyHere, requestDelete: requestDelete,
                        jumpToQuote: { commentID in jump(to: commentID, proxy: proxy) }
                    )
                    .id(row.id)
                }
            }
            .onChange(of: rows) { now = Date() }
        }
    }

    private func jump(to commentID: String, proxy: ScrollViewProxy) {
        guard let id = Self.quotedRowID(commentID: commentID, in: rows.map(\.message)) else { return }
        withAnimation(.easeOut(duration: 0.25)) { proxy.scrollTo(id, anchor: .center) }
    }
}

struct ChatThreadHead: View {
    let parent: ChatMessage
    let members: [Components.Schemas.CabalMember]
    var proposals: ChatProposalStore?
    let now: Date
    let openProfile: (String) -> Void
    var openProposal: ((String) -> Void)?

    static func proposalID(of parent: ChatMessage?) -> String? {
        guard let parent, !parent.deleted else { return nil }
        return parent.proposalId
    }

    var body: some View {
        if let proposalID = Self.proposalID(of: parent), let proposals {
            ChatThreadProposalHead(
                parent: parent, proposalID: proposalID, store: proposals, members: members, now: now,
                openProfile: openProfile, openProposal: { openProposal?($0) })
        } else {
            ChatThreadParent(parent: parent, members: members, openProfile: openProfile)
        }
    }
}

struct ChatThreadTitle: ViewModifier {
    let subtitle: String

    @ViewBuilder func body(content: Content) -> some View {
        if #available(iOS 26, *) {
            content
                .navigationTitle(ChatThreadCopy.title)
                .navigationSubtitle(subtitle)
                .navigationBarTitleDisplayMode(.inline)
        } else {
            content
                .navigationTitle(ChatThreadCopy.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .principal) { stacked }
                }
        }
    }

    private var stacked: some View {
        VStack(spacing: 0) {
            Text(ChatThreadCopy.title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
            if !subtitle.isEmpty {
                Text(subtitle)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
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
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(1)
                        .monacoPrivate()
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
                .font(MonacoTheme.Typo.subhead)
                .italic()
                .foregroundStyle(MonacoTheme.secondaryText)
        } else {
            ChatMessageText(
                text: parent.body ?? "", members: members, color: MonacoTheme.bodyText,
                mentionColor: MonacoTheme.brand, openProfile: openProfile)
            Text(parent.createdAt, format: .dateTime.hour().minute())
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("chat-thread-parent-time")
        }
    }
}
