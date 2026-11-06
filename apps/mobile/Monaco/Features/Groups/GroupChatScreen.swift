import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatScreen: View {
    let cabalID: String
    var embedded = false
    let session: ChatSession?
    let cabal: Components.Schemas.Cabal?
    var reporter: ChatSeenReporter?
    var viewerID = ""
    var proposals: ChatProposalStore?
    var openVotes: ChatOpenVotesModel?
    var network: NetworkPathModel?
    var gifs: (any GifProvider)?
    let openProfile: (String) -> Void
    let openThread: (String) -> Void
    var openProposal: (String) -> Void = { _ in }
    var reactionsOverride: ReactionsModel?

    @Environment(AppEnvironment.self) private var environment
    @State private var ownReactions: ReactionsModel?
    @State private var seenTarget: SeenTarget?
    @State private var showsDetails = false
    @State private var chat: ChatSession.State?
    @Environment(ToastCenter.self) private var toasts
    @State private var messageToDelete: String?
    @State private var visibleCards: Set<String> = []
    @State private var showsPoll = false
    @State private var quietFacts = ChatQuietFacts()
    @State private var now = Date()
    @Environment(AppEnvironment.self) private var appEnvironment: AppEnvironment?
    @Environment(\.stockSearch) private var stockSearch
    @FocusState private var composerFocused: Bool
    @Environment(\.scenePhase) private var scenePhase

    private var sessionID: ObjectIdentifier? { session.map(ObjectIdentifier.init) }
    private var openVotesID: ObjectIdentifier? { openVotes.map(ObjectIdentifier.init) }
    private var reporterID: ObjectIdentifier? { reporter.map(ObjectIdentifier.init) }
    private var closedWithoutMessages: Bool {
        chat?.isClosed == true && chat?.timeline.hasLoadedNewest != true
    }

    private var thesis: ChatThesisSource? {
        openVotes.flatMap { ChatThesisSource.newest(in: $0.votes) }
    }

    private var proposalCardIDs: Set<String> {
        Set(chat?.timeline.rows.compactMap(\.message.proposalId) ?? [])
    }

    private var title: String { GroupChatCopy.title(groupName: cabal?.name) }

    var body: some View {
        VStack(spacing: 0) {
            if network?.isOnline == false { ChatOfflineNotice() }
            messagesArea
            bottomBar
        }
        .monacoCanvas()
        .task {
            if ownReactions == nil { ownReactions = ReactionsModel(api: environment.api, clock: ContinuousClock()) }
        }
        .modifier(ChatStandaloneChrome(embedded: embedded, title: titleView, showsDetails: $showsDetails))
        .sheet(isPresented: $showsDetails) { ChatCabalDetailsSheet(cabalID: cabalID) }
        .modifier(ChatPollSheet(isPresented: $showsPoll) { draft in sendPoll(draft) })
        .environment(\.pollVoting, session?.pollVoting)
        .task { network?.start() }
        .task(id: sessionID) { await observe() }
        .task(id: sessionID) { await session?.open() }
        .task(id: openVotesID) { await openVotes?.load() }
        .task(id: openVotesID) { await openVotes?.observe(hints: environment.hints) }
        .onChange(of: proposalCardIDs) { old, new in
            if !new.subtracting(old).isEmpty { Task { await openVotes?.reload() } }
        }
        .task(id: quietKind == .fresh) { await loadQuietFacts() }
        .onChange(of: chat?.timeline.rows) { now = Date() }
        .task(id: reporterID) { await reporter?.setScreenVisible(true) }
        .onAppear { Task { await reporter?.setScreenVisible(true) } }
        .sheet(item: $seenTarget) { target in
            ChatSeenSheet(cabalID: cabalID, messageID: target.id, openProfile: openProfile)
        }
        .onChange(of: scenePhase) { _, phase in handle(phase) }
        .onChange(of: chat?.notice) { _, notice in show(notice) }
        .onDisappear {
            Task {
                await reporter?.setScreenVisible(false)
                await session?.close()
            }
        }
        .chatDeleteConfirmation(messageID: $messageToDelete) { id in delete(id) }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-view")
    }

    private var titleView: some View {
        Text(title)
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
            .truncationMode(.tail)
            .accessibilityAddTraits(.isHeader)
            .accessibilityIdentifier("chat-title")
    }

    @ViewBuilder private var messagesArea: some View {
        if let chat, chat.timeline.hasLoadedNewest {
            thread(chat)
        } else if closedWithoutMessages {
            EmptyState(title: GroupChatCopy.closed, isOnlyContent: true)
                .accessibilityIdentifier("chat-closed")
        } else if case .failed = chat?.load {
            GroupChatLoadFailureView { Task { await session?.reload() } }
        } else {
            ChatSkeleton()
        }
    }

    private func thread(_ chat: ChatSession.State) -> some View {
        ScrollViewReader { proxy in
            threadView(chat)
                .modifier(
                    ChatPinHost(
                        pins: chat.pins, cabalName: cabal?.name ?? "", viewerID: viewerID,
                        controls: ChatPinControls(pins: chat.pins, role: cabal?.me?.role, viewerID: viewerID),
                        isShown: network?.isOnline != false,
                        openVotes: openVotes, visibleCards: visibleCards, proposals: proposals,
                        goToVote: { proposalID in
                            switch ChatPinVoteTarget.resolve(proposalID: proposalID, rows: chat.timeline.rows) {
                            case .scroll(let id): withAnimation { proxy.scrollTo(id, anchor: .center) }
                            case .proposal(let id): openProposal(id)
                            }
                        },
                        scrollTo: { id in withAnimation { proxy.scrollTo(id, anchor: .center) } }))
        }
    }

    private func threadView(_ chat: ChatSession.State) -> some View {
        GroupChatThreadView(
            rows: chat.timeline.rows,
            members: cabal?.members ?? [],
            cabalName: cabal?.name ?? "",
            proposals: proposals,
            openProposal: openProposal,
            hasOlder: chat.timeline.hasOlder,
            isLoadingOlder: chat.isLoadingOlder,
            openProfile: openProfile,
            openThread: openThread,
            requestDelete: { messageToDelete = $0 },
            seen: chat.seen,
            openSeen: { seenTarget = SeenTarget(id: $0) },
            retry: { key in Task { await session?.retry(key: key) } },
            discard: { key in Task { await session?.discard(key: key) } },
            loadOlder: { Task { await session?.loadOlder() } },
            refresh: { await session?.reload() },
            reactions: reactionsOverride ?? ownReactions, cabalID: cabalID,
            quiet: quietContent,
            cardVisibility: { id, isVisible in
                if isVisible { visibleCards.insert(id) } else { visibleCards.remove(id) }
            }
        )
    }

    private var resolvedStockSearch: StockSearch? {
        stockSearch ?? appEnvironment.map { LiveStockSearch.make($0.api) }
    }

    private var quietKind: ChatQuietState.Kind {
        guard let chat, chat.timeline.hasLoadedNewest, chat.footer == .composer, let cabal else { return .none }
        return ChatQuietState.kind(rows: chat.timeline.rows, openProposals: Int(cabal.openProposalCount), now: now)
    }

    private var quietContent: ChatQuietContent? {
        guard let cabal else { return nil }
        return ChatQuietContent(
            kind: quietKind, cabalID: cabalID, cabalName: cabal.name,
            pictureURL: CabalMark.resolvedURL(cabal.pictureUrl),
            line: quietFacts.line(memberCount: Int(cabal.memberCount)),
            actions: ChatQuietState.actions(
                canPropose: cabal.me?.canPropose == true, voterCount: Int(cabal.voterCount)),
            search: resolvedStockSearch,
            focusComposer: { composerFocused = true }, openPoll: { presentPoll() },
            sendStock: { draft in Task { await session?.send(body: nil, attachment: draft) } })
    }

    private func loadQuietFacts() async {
        guard quietKind == .fresh, let appEnvironment else { return }
        if let facts = await ChatQuietFacts.load(api: appEnvironment.api, cabalID: cabalID) { quietFacts = facts }
    }

    @ViewBuilder private var bottomBar: some View {
        switch chat?.footer ?? .none {
        case .none:
            EmptyView()
        case .closedNotice:
            GroupChatClosedNotice()
        case .composer:
            ChatComposerBar(
                focus: $composerFocused,
                placeholder: ChatQuietState.composerPlaceholder(kind: quietKind, cabalName: cabal?.name),
                members: cabal?.members ?? [], viewerID: viewerID,
                attachments: attachments
            ) { body in
                Task { await session?.send(body: body) }
            }
        }
    }

    private var attachments: ChatComposerAttachments? {
        guard let provider = gifs ?? appEnvironment?.gifs else { return nil }
        return ChatComposerAttachments(
            gifs: provider,
            search: resolvedStockSearch,
            thesis: thesis
        ) { body, attachment in
            Task { await session?.send(body: body, attachment: attachment) }
        } presentPoll: {
            presentPoll()
        }
    }

    func presentPoll() {
        showsPoll = true
    }

    private func sendPoll(_ draft: ChatAttachmentDraft) {
        Task { await session?.send(body: nil, attachment: draft) }
    }

    private func observe() async {
        guard let session else { return }
        for await state in await session.states() {
            chat = state
        }
    }

    private func handle(_ phase: ScenePhase) {
        Task { await reporter?.setAppActive(phase == .active) }
        guard let session else { return }
        switch phase {
        case .background: Task { await session.close() }
        case .active: Task { await session.open() }
        default: return
        }
    }

    private func delete(_ id: String) {
        Task {
            let failure = await session?.delete(messageId: id)
            toasts.current = MonacoToast(message: failure.map(ToastCopy.message(for:)) ?? GroupChatCopy.deleted)
        }
    }

    private func show(_ notice: ChatSession.Notice?) {
        guard let notice else { return }
        toasts.current = MonacoToast(message: ToastCopy.message(for: notice.error))
    }
}

private struct ChatStandaloneChrome<Title: View>: ViewModifier {
    let embedded: Bool
    let title: Title
    @Binding var showsDetails: Bool

    @ViewBuilder func body(content: Content) -> some View {
        if embedded {
            content
        } else {
            content
                .navigationBarTitleDisplayMode(.inline)
                .toolbar(.hidden, for: .tabBar)
                .toolbar {
                    ToolbarItem(placement: .principal) { title }
                    ToolbarItem(placement: .topBarTrailing) {
                        MonacoGlassButton(
                            systemImage: MonacoIcon.info.symbol, accessibilityLabel: GroupChatCopy.infoLabel,
                            style: .quiet, placement: .toolbar
                        ) { showsDetails = true }
                        .accessibilityIdentifier("chat-info")
                    }
                }
        }
    }
}

private struct SeenTarget: Identifiable {
    let id: String
}
