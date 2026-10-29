import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

struct CatchUpView: View {
    @State private var model: CatchUpModel?
    private let now: @Sendable () -> Date

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.analytics) private var analytics

    init(model: CatchUpModel? = nil, now: @escaping @Sendable () -> Date = { Date() }) {
        _model = State(initialValue: model)
        self.now = now
    }

    var body: some View {
        content
            .monacoCanvas()
            .navigationTitle(CatchUpCopy.title)
            .navigationBarTitleDisplayMode(.large)
            .task { await start() }
            .onAppear { model?.setVisible(true) }
            .onDisappear { model?.setVisible(false) }
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
            sections(screen)
        }
    }

    private func sections(_ screen: CatchUpScreen) -> some View {
        let actions = CatchUpRowActions(vote: vote, open: open, join: join, clear: clear)
        let now = now()
        return List {
            if screen.isCaughtUp {
                CatchUpCaughtUp(cabals: screen.cabals, openFeed: openFeed)
                    .catchUpListRow()
                CatchUpSection(
                    slot: .cleared, title: CatchUpCopy.clearedHeader, items: screen.cleared, now: now, actions: actions)
            } else {
                Text(CatchUpCopy.subLine(count: screen.count))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.s)
                    .catchUpListRow()
                    .accessibilityIdentifier("catch-up-subline")
                CatchUpSection(
                    slot: .waiting, title: CatchUpCopy.waitingHeader, items: screen.waiting, now: now, actions: actions)
                CatchUpSection(
                    slot: .replies, title: CatchUpCopy.repliesHeader, items: screen.replies, now: now,
                    actions: actions)
                CatchUpSection(
                    slot: .invites, title: CatchUpCopy.invitesHeader, items: screen.invites, now: now,
                    actions: actions)
                CatchUpSection(
                    slot: .results, title: CatchUpCopy.resultsHeader, items: screen.results, now: now,
                    actions: actions)
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .accessibilityIdentifier("catch-up-page")
    }

    private func opened(_ item: Components.Schemas.CatchUpItem) {
        analytics.step(.catchUp(.openedItem), properties: ["kind": .string(item.kind.rawValue)])
        guard let model else { return }
        Task { await model.read(item.key) }
    }

    private func vote(_ item: Components.Schemas.CatchUpItem) {
        opened(item)
        if let id = item.proposal?.id { environment.navigator.presentVoteHold(proposalID: id, pick: nil) }
    }

    private func clear(_ item: Components.Schemas.CatchUpItem) {
        guard let model else { return }
        Task {
            if await model.clear(item.key) {
                analytics.step(.catchUp(.cleared), properties: ["kind": .string(item.kind.rawValue)])
            } else {
                toasts.current = MonacoToast(message: CatchUpCopy.clearFailedToast)
            }
        }
    }

    private func openFeed() {
        analytics.step(.catchUp(.openFeed))
        environment.navigator.selectedTab = .feed
    }

    private func open(_ item: Components.Schemas.CatchUpItem) {
        opened(item)
        switch item.kind {
        case .waitingVote, .result:
            if let id = item.proposal?.id { environment.navigator.open(ProposalRoute(proposalID: id), in: .home) }
        case .reply, .mention:
            if let id = item.proposal?.id {
                environment.navigator.open(ProposalRoute(proposalID: id), in: .home)
            } else if let id = item.post?.feedItemId {
                environment.navigator.open(FeedItemRoute(itemID: id), in: .home)
            }
        case .invite:
            if let id = item.cabal?.id { environment.navigator.open(CabalRoute(id: id), in: .home) }
        }
    }

    private func join(_ item: Components.Schemas.CatchUpItem) {
        guard let invite = item.invite, let cabal = item.cabal else { return }
        opened(item)
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
        await model.observe()
    }

    private func prepared() -> CatchUpModel {
        if let model { return model }
        let created = CatchUpModel(api: environment.api, hints: environment.hints, clock: now)
        model = created
        return created
    }
}
