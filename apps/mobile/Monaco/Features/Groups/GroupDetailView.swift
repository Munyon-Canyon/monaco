import MonacoAPI
import MonacoCore
import SwiftUI

/// Screens pushed from the group screen's action row and section headers.
enum GroupDetailRoute: Hashable {
    case chat
    case proposals
    case activity
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

/// Group screen: hero, action row, open votes, holdings, leaderboard, and activity.
/// Owns loading, polling, retry, and join-request state; `GroupDetailContent` is the layout.
struct GroupDetailView: View {
    @ObservedObject var auth: PrivyAuthService
    let groupId: String
    let groupName: String?
    let initialView: GroupViewDTO?

    private let apiClient = MonacoAPIClient()
    @Environment(AppSessionStore.self) private var session: AppSessionStore?

    @State private var groupView: GroupViewDTO?
    /// One idempotency key holder per transaction being retried; retries of different rows overlap.
    @State private var retrySubmissions: [String: IdempotentSubmission] = [:]
    @State private var activityItems: [GroupActivityItemDTO] = []
    @State private var activityLoading = true
    @State private var activityError: String?
    @State private var retryingTransactionIDs: Set<String> = []
    @State private var errorMessage: String?
    @State private var toast: MonacoToast?
    @State private var isLoading: Bool
    /// The blocking first load has run at least once; re-appearing is the poll loop's job.
    @State private var didInitialLoad = false
    @State private var proposalService: LiveProposalFeedService
    /// The pot's curve on the hero: one slot per range, re-read with the rest of the screen.
    @State private var pnl: GroupPnLHistoryModel

    @State private var proposalRefreshCount = 0
    @State private var route: GroupDetailRoute?
    @State private var showProposeSheet = false
    /// The propose sheet's height; the chooser raises it to `.large` while a flow is pushed.
    @State private var showDetailsSheet = false
    @State private var heroScrolledAway = false

    /// Whether the open-votes preview has a proposal collecting votes right now.
    @State private var hasOpenVotes = false
    /// A vote closed within the settling window, so its outcome is still on its way to the pot.
    @State private var isWatchingVoteOutcome = false
    /// Bumped each time the last open vote closes; drives the settling-window timer.
    @State private var voteOutcomeWatch = 0
    /// Pull-to-refresh and the background poll share it, so a tick stands down while the member
    /// is refreshing by hand.
    @State private var refreshGate = RefreshGate()

    /// Votes land and swaps settle in seconds; a quiet cabal only needs its balances kept current.
    /// A deposit on its way into the pot is watched at the sweep cadence so the pot updates as it lands.
    private var pollInterval: Duration {
        GroupDetailCadence.interval(
            for: GroupDetailCadence.Inputs(
                hasOpenVotes: hasOpenVotes,
                hasPendingSwap: activityItems.contains { $0.status.lowercased() == "pending" },
                hasPendingDeposit: activityHasPendingDeposits,
                isWatchingVoteOutcome: isWatchingVoteOutcome
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
        _proposalService = State(initialValue: LiveProposalFeedService(auth: auth))
        _pnl = State(
            initialValue: GroupPnLHistoryModel(groupId: groupId, source: LiveGroupPnLHistorySource(auth: auth)))
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
            // A range chip on the hero: read that window if it has not been read yet.
            .onChange(of: pnl.range) { _, range in
                Task { await pnl.load(range: range) }
            }

            // A vote that just closed is still landing: keep watching closely for a little while.
            .task(id: voteOutcomeWatch) {
                guard voteOutcomeWatch > 0 else { return }
                isWatchingVoteOutcome = true
                defer { isWatchingVoteOutcome = false }
                try? await Task.sleep(for: GroupDetailCadence.voteSettlingWindow)
            }
            .refreshable {
                await refreshGate.runNow {
                    proposalRefreshCount += 1
                    try? await refresh(.userInitiated)
                }
            }
            .sheet(
                isPresented: $showProposeSheet,
                onDismiss: {
                    proposalRefreshCount += 1
                }
            ) {
                if let groupView {
                    ProposeSheet(auth: auth, groupId: groupId, groupView: groupView, onProposed: proposalSent)
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
                proposalService: proposalService,
                proposalRefreshToken: "\(proposalRefreshCount)",
                onOpenVotesChange: openVotesChanged,
                activityItems: activityItems,
                activityLoading: activityLoading,
                activityError: activityError,
                retryingTransactionIDs: retryingTransactionIDs,
                onRoute: { route = $0 },
                onPropose: { showProposeSheet = true },
                onRetry: { item in Task { await retryTransaction(item) } },
                onToast: { toast = $0 },
                onHeroScrolledAway: { heroScrolledAway = $0 },
                heroChart: pnl.chart,
                heroRange: pnl.range,
                onHeroRange: { pnl.range = $0 }
            )
        } else if let errorMessage {

            statusCard {
                Text(errorMessage)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                Button("Try again") {
                    Task { await refreshGate.runNow { try? await refresh(.initial) } }
                }
                .buttonStyle(.monacoSecondary)
            }
            .accessibilityIdentifier("group-detail-error")
        } else if isLoading {
            GroupDetailSkeleton()
        } else {
            statusCard {
                Text("Couldn't load this cabal. Pull down to try again")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                Button("Try again") {
                    Task { await refreshGate.runNow { try? await refresh(.initial) } }
                }
                .buttonStyle(.monacoSecondary)
            }
        }
    }

    /// Scrollable so pull-to-refresh works from the error state too.
    private func statusCard<Content: View>(@ViewBuilder content: () -> Content) -> some View {
        ScrollView {
            VStack(spacing: 16) {
                content()
            }
            .padding(24)
            .frame(maxWidth: .infinity)
            .padding(.top, 48)
        }
    }

    @ViewBuilder
    private func destination(for route: GroupDetailRoute) -> some View {
        switch route {
        case .chat:
            GroupChatView(auth: auth, groupId: groupId, groupName: displayName)
        case .proposals:
            ProposalFeedView(service: proposalService, groupId: groupId)
        case .activity:
            GroupActivityListView(
                auth: auth,
                items: activityItems,
                retryingTransactionIDs: retryingTransactionIDs,
                onRetry: { item in Task { await retryTransaction(item) } }
            )
        case .stock(let symbol):
            AssetDetailClientView(symbol: symbol)
        }
    }

    private var loadTaskID: String {
        "\(groupId)-\(auth.accessToken ?? "")"
    }

    /// The open-votes preview reporting what it is showing.
    ///
    /// A proposal leaves the open list the moment it passes, which is exactly when the swap it
    /// decided starts. Read the cabal straight away and keep watching closely for a while, so the
    /// pot, holdings and activity move while the member is still looking at the vote they cast.
    private func openVotesChanged(_ nowOpen: Bool) {
        let votesJustClosed = hasOpenVotes && !nowOpen
        hasOpenVotes = nowOpen
        guard votesJustClosed else { return }
        voteOutcomeWatch += 1
        Task { await refreshQuietly() }
    }

    /// Called by the propose sheet once the cabal has the proposal: close the sheet and confirm.
    /// Closing the sheet reloads the open votes.
    private func proposalSent(_ proposalId: String) {
        showProposeSheet = false
        Haptics.success()
        toast = MonacoToast(message: "Proposal sent to \(displayName)", isSuccess: true)
    }

    /// The one way this screen reads itself.
    ///
    /// The cabal, its activity and — for an admin — the people waiting to join are read together
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
                activityLoading = false
                errorMessage = "Sign in again to see this cabal."
            }
            return
        }
        if mode == .initial {
            isLoading = true
            activityLoading = true
        }
        if mode != .quiet {
            errorMessage = nil
            activityError = nil
        }
        defer {
            if mode == .initial {
                isLoading = false
                activityLoading = false
            }
        }

        async let viewLoad = apiClient.getGroupView(accessToken: token, groupId: groupId)
        async let activityLoad = apiClient.getGroupActivity(accessToken: token, groupId: groupId)
        // The curve rides along with the rest of the read; its failures are its own, and a
        // quiet one leaves the drawn curve alone.
        async let curveLoad: Void = pnl.load(range: pnl.range, quietly: mode == .quiet)

        let activity = try? await activityLoad
        await curveLoad

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
            if errorMessage != nil { errorMessage = nil }
        }
        if let activity {
            QuietUpdate.apply(activity.items, over: activityItems) { activityItems = $0 }
            if activityError != nil { activityError = nil }
        } else if mode != .quiet, activityItems.isEmpty {
            activityError = "Couldn't load activity. Pull down to try again"
        }

        guard let viewFailure, !viewFailure.isRequestCancellation else { return }
        if mode == .quiet { throw viewFailure }
        if groupView == nil {
            errorMessage = "Couldn't load this cabal. Pull down to try again"
        } else if mode == .userInitiated {
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

    private var activityHasPendingDeposits: Bool {
        activityItems.contains { item in
            item.kind.lowercased() == "deposit" && DepositStatusNormalizer.isPending(item.status)
        }
    }

    private func retryTransaction(_ item: GroupActivityItemDTO) async {
        guard let token = auth.accessToken else {
            toast = MonacoToast(message: "Sign in again to retry.")
            return
        }
        guard !retryingTransactionIDs.contains(item.id) else { return }

        retryingTransactionIDs.insert(item.id)
        defer { retryingTransactionIDs.remove(item.id) }

        let submission = retrySubmissions[item.id] ?? IdempotentSubmission()
        retrySubmissions[item.id] = submission

        do {
            let result = try await apiClient.retryTransaction(
                accessToken: token, transactionId: item.id, submission: submission)
            await refreshQuietly()
            if result.status.lowercased() == "confirmed" {
                let done = item.kind.lowercased() == "sell" ? "Sold" : "Bought"
                toast = MonacoToast(message: "\(done). Holdings updated", isSuccess: true)
            } else if result.status.lowercased() == "failed" {
                toast = MonacoToast(message: "It didn't go through again. Try later")
            }
        } catch is CancellationError {
            return
        } catch MonacoAPIError.httpStatus(let code) where code == 409 {
            toast = MonacoToast(message: "This one can't be retried")
        } catch MonacoAPIError.httpStatus {
            toast = MonacoToast(message: "Retry didn't go through. Try again")
        } catch {
            toast = MonacoToast(message: "Retry didn't go through. Try again")
        }
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
    let proposalService: ProposalFeedService
    let proposalRefreshToken: String
    var onOpenVotesChange: (Bool) -> Void = { _ in }
    let activityItems: [GroupActivityItemDTO]
    let activityLoading: Bool
    let activityError: String?
    let retryingTransactionIDs: Set<String>
    let onRoute: (GroupDetailRoute) -> Void
    let onPropose: () -> Void
    let onRetry: (GroupActivityItemDTO) -> Void
    let onToast: (MonacoToast) -> Void
    var onHeroScrolledAway: (Bool) -> Void = { _ in }
    /// Nil on read-only surfaces; the hero then draws a plain mark.
    var pictureEditor: CabalPictureEditor?
    /// The pot's curve on the hero, owned by the screen.
    var heroChart: GroupHeroChart = .loading
    var heroRange: GroupPnLRange = .oneMonth
    var onHeroRange: (GroupPnLRange) -> Void = { _ in }

    var body: some View {
        ScrollView {
            // No horizontal padding on the stack: the hero band and the ruled lists run edge to
            // edge, and each section insets its own header.
            LazyVStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                VStack(spacing: MonacoTheme.Space.l) {
                    GroupHeroSection(
                        view: view,
                        chart: heroChart,
                        range: heroRange,
                        onRange: onHeroRange,
                        pictureEditor: pictureEditor,
                        onPictureResult: onToast
                    )
                    GroupActionRow(onRoute: onRoute, onPropose: onPropose)
                        .padding(.horizontal, MonacoTheme.Space.m)
                }

                VStack(alignment: .leading, spacing: 0) {
                    ProposalHistorySection(
                        service: proposalService,
                        groupId: view.id,
                        refreshToken: proposalRefreshToken,
                        onOpenVotesChange: onOpenVotesChange,
                        onSeeAll: { onRoute(.proposals) },
                        onToast: onToast
                    )
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

                MemberBoardSection(members: view.members, currentUserId: currentUserId)

                GroupActivitySection(
                    auth: auth,
                    items: activityItems,
                    isLoading: activityLoading,
                    errorMessage: activityError,
                    retryingTransactionIDs: retryingTransactionIDs,
                    onRetry: onRetry,
                    onSeeAll: { onRoute(.activity) }
                )
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
            .padding(.horizontal, MonacoTheme.Space.m)

            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 120, height: 22)
                ForEach(0..<3, id: \.self) { _ in
                    SkeletonBlock(height: 60, radius: 0)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
        }
        .padding(.top, 8)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading cabal")
        .accessibilityIdentifier("group-detail-loading")
    }
}
