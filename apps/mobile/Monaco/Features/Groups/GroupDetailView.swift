import MonacoAPI
import MonacoCore
import SwiftUI

/// Screens pushed from the group screen's action row and section headers.
enum GroupDetailRoute: Hashable {
    case chat
    /// A holding on the cabal screen, opened as the stock it is. The row already
    /// shows the day's shape and its change; tapping it should go where those
    /// numbers come from rather than being the end of the road.
    case stock(symbol: String)
}

/// What a refresh of the cabal screen is allowed to show while it runs.
private nonisolated enum GroupDetailRefreshMode {
    /// The screen has nothing yet: a skeleton while it waits, and an error if it fails.
    case initial
    /// The member pulled down. No skeleton over content they can already see, but a failure is
    /// theirs to hear about.
    case userInitiated
    /// Nobody asked. Write what changed, say nothing, and leave the screen alone on failure.
    case quiet
}

/// Group screen: hero, action row, open votes, holdings and leaderboard.
/// Owns loading, polling and join-request state; `GroupDetailContent` is the layout.
struct GroupDetailView: View {
    @ObservedObject var auth: PrivyAuthService
    let groupId: String
    let groupName: String?
    let initialView: GroupViewDTO?

    private let apiClient = MonacoAPIClient()
    @Environment(AppSessionStore.self) private var session: AppSessionStore?
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?

    @State private var groupView: GroupViewDTO?
    @State private var toast: MonacoToast?
    @State private var isLoading: Bool
    /// The blocking first load has run at least once; re-appearing is the poll loop's job.
    @State private var didInitialLoad = false
    @State private var route: GroupDetailRoute?
    @State private var showDetailsSheet = false
    @State private var heroScrolledAway = false

    /// Pull-to-refresh and the background poll share it, so a tick stands down while the member
    /// is refreshing by hand.
    @State private var refreshGate = RefreshGate()

    /// Votes land and swaps settle in seconds; a quiet cabal only needs its balances kept current.
    /// A deposit on its way into the pot is watched at the sweep cadence so the pot updates as it lands.
    private var pollInterval: Duration {
        GroupDetailCadence.interval(
            for: GroupDetailCadence.Inputs(
                hasPendingSwap: false,
                hasPendingDeposit: false
            ))
    }

    init(
        auth: PrivyAuthService,
        groupId: String,
        groupName: String? = nil,
        initialView: GroupViewDTO? = nil
    ) {
        self.auth = auth
        self.groupId = groupId
        self.groupName = groupName
        self.initialView = initialView
        _isLoading = State(initialValue: initialView == nil)
    }

    private var displayName: String {
        groupView?.name ?? groupName ?? "Cabal"
    }

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            // The hero carries the name; the bar only shows it once the hero scrolls away.
            .navigationTitle(groupView == nil || heroScrolledAway ? displayName : "")
            .navigationBarTitleDisplayMode(.inline)
            .cabalHeroNavigationBar(isOverHero: groupView != nil && !heroScrolledAway)

            .toolbar {
                if groupView != nil {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            showDetailsSheet = true
                        } label: {
                            Image(systemName: "info.circle")
                        }
                        .accessibilityLabel("Cabal details")
                        .accessibilityIdentifier("group-details-button")
                    }
                }
            }
            .navigationDestination(item: $route) { route in
                destination(for: route)
            }
            // The first appearance loads; coming back from a pushed screen does not. The poll
            // loop below already knows how stale its data is and ticks straight away when it is.
            .task(id: loadTaskID) {
                if let initialView, groupView == nil {
                    groupView = initialView
                    isLoading = false
                }
                guard !didInitialLoad || groupView == nil else { return }
                didInitialLoad = true
                try? await refreshGate.runNow { try await refresh(.initial) }
            }
            .pollWhileVisible(every: pollInterval, isActive: groupView != nil, gate: refreshGate) {
                try await refresh(.quiet)
            }
            .refreshable {
                await refreshGate.runNow {
                    try? await refresh(.userInitiated)
                }
            }
            .sheet(isPresented: $showDetailsSheet) {
                if let groupView {
                    GroupDetailsSheet(treasuryAddress: groupView.treasuryAddress)
                }
            }
            .monacoToast($toast)
    }

    @ViewBuilder
    private var content: some View {
        if let groupView {
            GroupDetailContent(
                auth: auth,
                view: groupView,
                currentUserId: session?.profile?.userID,
                onRoute: { route = $0 },
                onPropose: {
                    guard let environment else { return }
                    environment.navigator.open(ProposeRoute(cabalID: groupId), in: environment.navigator.selectedTab)
                },
                onToast: { toast = $0 },
                onHeroScrolledAway: { heroScrolledAway = $0 }
            )
        } else if isLoading {
            GroupDetailSkeleton()
        } else {
            errorState
        }
    }

    /// Scrollable so pull-to-refresh works from the error state too.
    private var errorState: some View {
        ScrollView {
            MonacoErrorRow(thing: "this cabal", identifier: "group-detail-error") {
                Task { await refreshGate.runNow { try? await refresh(.initial) } }
            }
            .padding(.top, MonacoTheme.Space.xl)
        }
    }

    @ViewBuilder
    private func destination(for route: GroupDetailRoute) -> some View {
        switch route {
        case .chat:
            ChatRoute(cabalID: groupId).destination()
        case .stock(let symbol):
            AssetDetailClientView(symbol: symbol)
        }
    }

    private var loadTaskID: String {
        "\(groupId)-\(auth.accessToken ?? "")"
    }

    /// The one way this screen reads itself.
    ///
    /// The cabal and — for an admin — the people waiting to join are read together
    /// rather than one after another, and written through `QuietUpdate`, so a refresh that finds
    /// nothing new changes nothing the member can see. Every caller goes through `refreshGate`,
    /// which is what stops a resuming poll tick from racing the appear load back into the view
    /// with two near-identical snapshots: a tick asks with `run` and is dropped while anything
    /// else holds the gate. It does not serialise the reads the member asks for — `runNow` marks
    /// the gate busy but waits for nothing — so two of those (a pull-to-refresh over the
    /// follow-up to a retry, say) can still be in flight together and land last-writer-wins.
    ///
    /// Failure belongs to whoever asked. A `.quiet` read rethrows so the poll loop backs off and
    /// leaves the screen exactly as the member last saw it; the other modes say so.
    private func refresh(_ mode: GroupDetailRefreshMode) async throws {
        guard let token = auth.accessToken else {
            if mode != .quiet {
                isLoading = false
            }
            return
        }
        if mode == .initial {
            isLoading = true
        }
        defer {
            if mode == .initial {
                isLoading = false
            }
        }

        async let viewLoad = apiClient.getGroupView(accessToken: token, groupId: groupId)
        var loadedView: GroupViewDTO?
        var viewFailure: Error?
        do {
            loadedView = try await viewLoad
        } catch {
            viewFailure = error
        }

        guard !Task.isCancelled else { return }

        if let loadedView {
            QuietUpdate.apply(loadedView, over: groupView) { groupView = $0 }
        }

        guard let viewFailure, !viewFailure.isRequestCancellation else { return }
        if mode == .quiet { throw viewFailure }
        if groupView != nil, mode == .userInitiated {
            toast = MonacoToast(message: "Couldn't refresh this cabal. Try again")
        }
    }

    /// The read that follows something the member just did — funding, cashing out, answering a
    /// request, retrying a swap, a vote closing. It goes through the gate (so a poll tick that
    /// lands on top of it stands down) but is never itself dropped, and it shows nothing either
    /// way: the action it follows has already said what happened.
    private func refreshQuietly() async {
        try? await refreshGate.runNow { try await refresh(.quiet) }
    }
}

extension View {
    /// The bar over the cabal hero is the hero's own ink, so the band runs from the status bar
    /// down; once the band has scrolled away the bar goes back to the paper.
    func cabalHeroNavigationBar(isOverHero: Bool) -> some View {
        toolbarBackground(MonacoTheme.heroInk, for: .navigationBar)
            .toolbarBackground(isOverHero ? .visible : .hidden, for: .navigationBar)
            .toolbarColorScheme(isOverHero ? .dark : nil, for: .navigationBar)
            .animation(.easeInOut(duration: 0.2), value: isOverHero)
    }

}

/// Scrollable layout of the group screen. Pure: data in, actions out.
struct GroupDetailContent: View {
    @ObservedObject var auth: PrivyAuthService
    let view: GroupViewDTO
    let currentUserId: String?
    let onRoute: (GroupDetailRoute) -> Void
    let onPropose: () -> Void
    let onToast: (MonacoToast) -> Void
    var onHeroScrolledAway: (Bool) -> Void = { _ in }
    /// Nil on read-only surfaces; the hero then draws a plain mark.
    var pictureEditor: CabalPictureEditor?

    var body: some View {
        ScrollView {
            // No horizontal padding on the stack: the hero band and the ruled lists run edge to
            // edge, and each section insets its own header.
            LazyVStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                VStack(spacing: MonacoTheme.Space.l) {
                    GroupHeroSection(
                        view: view,
                        pictureEditor: pictureEditor,
                        onPictureResult: onToast
                    )
                    GroupActionRow(onRoute: onRoute, onPropose: onPropose)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                }

                VStack(alignment: .leading, spacing: 0) {
                    PotSectionView(
                        pot: view.pot,
                        groupId: view.id,
                        onOpenStock: { onRoute(.stock(symbol: $0)) }
                    )
                }

                if let agent = view.agent {
                    AgentSectionView(agent: agent) { message in
                        onToast(MonacoToast(message: message, isSuccess: true))
                    }
                }
            }
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollIndicators(.hidden)
        .onScrollGeometryChange(for: Bool.self) { geometry in
            geometry.contentOffset.y + geometry.contentInsets.top > 140
        } action: { _, scrolledAway in
            onHeroScrolledAway(scrolledAway)
        }
    }
}

/// Propose · Chat, directly under the hero.
struct GroupActionRow: View {
    let onRoute: (GroupDetailRoute) -> Void
    let onPropose: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            action("Propose", systemImage: "arrow.up.right", id: "group-action-propose", perform: onPropose)
            action("Chat", systemImage: "bubble.left", id: "group-action-chat") { onRoute(.chat) }
        }
    }

    private func action(_ title: String, systemImage: String, id: String, perform: @escaping () -> Void) -> some View {
        CircleAction(title, systemImage: systemImage, action: perform)
            .frame(maxWidth: .infinity)
            .accessibilityIdentifier(id)
    }
}

struct GroupDetailSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
            // The band, in its own shape.
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(width: 48, height: 48, radius: MonacoTheme.Radius.tile)
                    SkeletonBlock(width: 180, height: 22)
                }
                SkeletonBlock(width: 200, height: 44)
                SkeletonBlock(height: 76, radius: 0)
                SkeletonBlock(width: 140, height: 18)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MonacoTheme.heroInk.opacity(0.08))

            HStack {
                ForEach(0..<4, id: \.self) { _ in
                    SkeletonBlock(width: 56, height: 56, radius: 28)
                        .frame(maxWidth: .infinity)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)

            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 120, height: 22)
                ForEach(0..<3, id: \.self) { _ in
                    SkeletonBlock(height: 60, radius: 0)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .padding(.top, 8)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading cabal")
        .accessibilityIdentifier("group-detail-loading")
    }
}
