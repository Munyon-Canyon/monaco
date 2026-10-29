import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

struct CatchUpView: View {
    @State private var model: CatchUpModel?
    @State private var deckStart: CatchUpDeckStart?
    private let opensDeckAt: CatchUpDeckStart?
    private let now: @Sendable () -> Date

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?

    init(
        model: CatchUpModel? = nil, now: @escaping @Sendable () -> Date = { Date() },
        opensDeckAt: CatchUpDeckStart? = nil
    ) {
        _model = State(initialValue: model)
        self.opensDeckAt = opensDeckAt
        self.now = now
    }

    var body: some View {
        content
            .monacoCanvas()
            .navigationTitle(CatchUpCopy.title)
            .navigationBarTitleDisplayMode(.large)
            .toolbar { oneByOne }
            .fullScreenCover(item: $deckStart) { start in deck(start) }
            .task { await start() }
            .onAppear { model?.setVisible(true) }
            .onDisappear { model?.setVisible(false) }
    }

    @ToolbarContentBuilder private var oneByOne: some ToolbarContent {
        if case .loaded(let screen) = model?.state, !screen.isCaughtUp {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    deckStart = CatchUpDeckStart(key: nil)
                } label: {
                    Label(CatchUpCopy.oneByOne, systemImage: "square.stack").labelStyle(.titleAndIcon)
                }
                .accessibilityIdentifier("catch-up-one-by-one")
            }
        }
    }

    @ViewBuilder private func deck(_ start: CatchUpDeckStart) -> some View {
        if let model, case .loaded(let screen) = model.state {
            CatchUpCardView(
                model: model, screen: screen, start: start, now: now(), rowActions: rowActions,
                reactions: ReactionsModel(api: environment.api, clock: ContinuousClock()),
                seeAll: {})
        }
    }

    private var rowActions: CatchUpRowActions {
        CatchUpRowActions(vote: vote, open: open, join: join, clear: clear)
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 5, markShape: .circle)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(CatchUpCopy.loadingLabel)
                .accessibilityIdentifier("catch-up-loading")
        case .failed:
            ScreenLoadError(identifier: "catch-up-error") { Task { await model?.load() } }
        case .loaded(let screen):
            List { sections(screen) }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .accessibilityIdentifier("catch-up-page")
        }
    }

    @ViewBuilder private func sections(_ screen: CatchUpScreen) -> some View {
        let actions = CatchUpRowActions(
            vote: vote, open: open, join: join, clear: clear)
        if screen.isCaughtUp {
            CatchUpCaughtUp(screen: screen, openFeed: openFeed).catchUpListRow()
        } else {
            Text(CatchUpCopy.subLine(count: screen.count))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.s)
                .accessibilityIdentifier("catch-up-subline")
                .catchUpListRow()
            CatchUpSection(
                title: CatchUpCopy.waitingHeader, identifier: "waiting", items: screen.waiting, now: now(),
                actions: actions)
            CatchUpSection(
                title: CatchUpCopy.repliesHeader, identifier: "replies", items: screen.replies, now: now(),
                actions: actions)
            CatchUpSection(
                title: CatchUpCopy.invitesHeader, identifier: "invites", items: screen.invites, now: now(),
                actions: actions)
            CatchUpSection(
                title: CatchUpCopy.resultsHeader, identifier: "results", items: screen.results, now: now(),
                actions: actions)
        }
        CatchUpSection(
            title: CatchUpCopy.clearedHeader, identifier: "cleared", items: screen.cleared, now: now(), actions: nil)
    }

    private func markRead(_ item: Components.Schemas.CatchUpItem) {
        let model = model
        Task { _ = await model?.read(item.key) }
    }

    private func vote(_ item: Components.Schemas.CatchUpItem) {
        markRead(item)
        guard let id = item.proposal?.id else { return }
        environment.navigator.presentVoteHold(proposalID: id, pick: nil)
    }

    private func clear(_ item: Components.Schemas.CatchUpItem) {
        let model = model
        AppAnalytics.current.step(.catchUp(.cleared), properties: ["kind": .string(item.kind.rawValue)])
        Task {
            let cleared = await model?.clear(item.key) ?? true
            if !cleared { toasts.current = MonacoToast(message: CatchUpCopy.clearFailedToast) }
        }
    }

    private func openFeed() {
        AppAnalytics.current.step(.catchUp(.openFeed))
        environment.navigator.selectedTab = .feed
    }

    private func open(_ item: Components.Schemas.CatchUpItem) {
        markRead(item)
        AppAnalytics.current.step(.catchUp(.openedItem), properties: ["kind": .string(item.kind.rawValue)])
        switch item.kind {
        case .waitingVote, .result:
            if let id = item.proposal?.id { environment.navigator.open(ProposalRoute(proposalID: id), in: .home) }
        case .reply, .mention:
            if let destination = ReplyDestination(catchUpItem: item) {
                environment.navigator.open(reply: destination)
            } else if let id = item.proposal?.id {
                environment.navigator.open(ProposalRoute(proposalID: id), in: .home)
            } else if let id = item.post?.feedItemId {
                environment.navigator.open(FeedItemRoute(itemID: id), in: .home)
            }
        case .invite:
            if let id = item.cabal?.id { environment.navigator.open(CabalRoute(id: id), in: .home) }
        }
    }

    private func join(_ item: Components.Schemas.CatchUpItem) {
        markRead(item)
        guard let invite = item.invite, let cabal = item.cabal else { return }
        let invites = CabalInvitesModel(api: environment.api, hints: environment.hints)
        Task {
            let received = ReceivedCabalInvite(requestID: invite.id, cabalID: cabal.id, cabalName: cabal.name)
            guard await invites.accept(received) else { return }
            toasts.current = MonacoToast(message: CatchUpCopy.joinedToast(cabal.name), isSuccess: true)
            await model?.load()
        }
    }

    private func start() async {
        let model = prepared()
        refresh?.register("catch-up") { await model.load() }
        if case .idle = model.state { await model.load() }
        if let opensDeckAt, deckStart == nil {
            deckStart = opensDeckAt
        }
        await model.observe()
    }

    private func prepared() -> CatchUpModel {
        if let model { return model }
        let created = CatchUpModel(api: environment.api, hints: environment.hints, clock: now)
        model = created
        return created
    }
}
