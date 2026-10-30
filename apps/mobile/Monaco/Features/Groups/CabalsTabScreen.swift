import MonacoCore
import SwiftUI

struct CabalsTabScreen: View {
    static let sections: [any CabalsTabSection.Type] = [
        CabalsJoinSlot.self,
        CabalsVoteSlot.self,
        CabalsGridSlot.self,
        CabalsShareListSlot.self,
    ]

    let sections: [any CabalsTabSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.accountRestricted) private var accountRestricted
    @State private var store: CabalsTabStore?

    init(sections: [any CabalsTabSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let live = SectionStack<Void>.live(sections.map { $0.erased })
        Group {
            if live.isEmpty {
                NotMigratedView(screen: "Cabals")
            } else if let store {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(Array(live.enumerated()), id: \.offset) { _, section in
                            AnyView(section.body(for: ()))
                        }
                        emptyState(store)
                    }
                    .padding(.bottom, MonacoTheme.Space.l)
                }
                .environment(store)
                .environment(\.homeVoteNeeds, store.needs)
                .onChange(of: store.content?.tiles.map(\.id)) { _, _ in store.syncOrder() }
                .onChange(of: store.order.failureTick) { _, _ in
                    if let error = store.order.lastError { toasts.show(error) }
                }
                .onChange(of: store.tab.failureTick) { _, _ in
                    guard store.content != nil, let error = store.tab.lastError else { return }
                    toasts.show(error)
                }
            } else {
                ProgressView()
            }
        }
        .task { await start() }
        .onAppear {
            guard let store, store.content == nil, !store.hasFailed else { return }
            Task { await store.tab.load() }
        }
    }

    @ViewBuilder private func emptyState(_ store: CabalsTabStore) -> some View {
        if let content = store.content, content.isEmpty {
            EmptyState(
                title: "Your cabal starts here",
                message: accountRestricted
                    ? "Open a shared invitation to join a cabal." : "Create a cabal or open a shared invitation.",
                actionTitle: accountRestricted ? nil : "Create a cabal",
                systemImage: MonacoIcon.cabals.symbol,
                secondaryActionTitle: accountRestricted ? nil : "Explore cabals",
                action: accountRestricted ? nil : { environment.navigator.open(CreateCabalRoute(), in: .cabals) },
                secondaryAction: accountRestricted
                    ? nil : { environment.navigator.open(DiscoverCabalsRoute(query: ""), in: .cabals) }
            )
            .accessibilityIdentifier("cabals-list-empty")
        } else if store.hasFailed {
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "cabals-list-retry",
                systemImage: MonacoIcon.offline.symbol,
                action: { Task { await store.tab.load() } }
            )
        }
    }

    private func start() async {
        let store = prepared()
        refresh?.register("cabals-tab") {
            await store.tab.load()
            await store.votes.load()
        }
        async let votes: Void = loadVotes(store)
        async let hints: Void = store.tab.observe(hints: environment.hints)
        await store.tab.load()
        store.syncOrder()
        _ = await (votes, hints)
    }

    private func loadVotes(_ store: CabalsTabStore) async {
        await store.votes.load()
        await store.votes.observe()
    }

    private func prepared() -> CabalsTabStore {
        if let store { return store }
        let created = CabalsTabStore(api: environment.api, hints: environment.hints, portfolio: portfolio)
        store = created
        return created
    }
}
