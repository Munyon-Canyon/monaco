import MonacoAPI
import MonacoCore
import SwiftUI

/// Cabals tab: P&L of your cabals, search, your cabals strip, and the
/// platform-wide board.
struct CabalsTabView: View {
    @ObservedObject var auth: PrivyAuthService
    @Environment(AppSessionStore.self) private var session
    @Environment(\.accountRestricted) private var accountRestricted
    @State private var model: CabalsTabModel
    @State private var searchText = ""
    @State private var showNewCabalSheet = false
    /// The pushed screen, if any. One item for the whole tab: see `CabalsRoute`.
    @State private var route: CabalsRoute?
    /// Chosen in the "New cabal" sheet, pushed once the sheet is gone. Pushing
    /// in the same turn as the dismissal makes the stack change mid-transition.
    @State private var routeAfterSheet: CabalsRoute?
    /// Starts true: the tab asks for the cabals list in `.task`, so on the very
    /// first body evaluation a load is about to happen. Starting at false made
    /// `stripState` compute `.unavailable` and render the hard error for a frame
    /// before anything had even been attempted.
    @State private var isLoadingCabals = true

    private let actions: CabalsActionSource

    init(
        auth: PrivyAuthService,
        dataSource: CabalsTabDataSource? = nil,
        actions: CabalsActionSource
    ) {
        self.auth = auth
        self.actions = actions
        _model = State(initialValue: CabalsTabModel(dataSource: dataSource ?? LiveCabalsTabDataSource(auth: auth)))
    }

    /// Membership as a set: the board reordering its rows is not a membership change.
    private var joinedIDs: Set<String> {
        Set(session.joinedCabals.map(\.groupId))
    }

    /// "No cabals yet" is only true once we have actually heard from the server.
    /// Until then the strip says it is loading, or offers a retry. The shell's
    /// own load counts: the state machine must never be able to claim a failure
    /// before an attempt has finished.
    private var stripState: CabalsStripState {
        if session.home != nil { return .loaded }
        return isLoadingCabals || session.isLoading ? .loading : .unavailable
    }

    var body: some View {
        MonacoScreen {
            ScrollView {
                // Edge to edge: the strip and the ruled lists run to the screen's edges, and
                // each section insets its own header.
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                    MonacoSearchField(placeholder: "Find a cabal by name", text: $searchText)
                        .padding(.horizontal, MonacoTheme.Space.m)
                        .accessibilityIdentifier("cabals-search-field")

                    if model.isSearching {

                        CabalsSearchResultsSection(model: model, onSelect: { route = $0 })
                    } else {
                        CabalsStripSection(
                            rows: session.joinedCabals,
                            state: stripState,
                            onSelect: { route = $0 },
                            onRetry: { Task { await loadCabals() } }
                        )
                        CabalsPnLChartSection(model: model, hasCabals: !session.joinedCabals.isEmpty)
                        CabalsLeaderboardSection(model: model, onSelect: { route = $0 })
                    }
                }
                .padding(.bottom, MonacoTheme.Space.xl)
            }

            .scrollDismissesKeyboard(.interactively)
        }
        .navigationTitle("Cabals")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            if !accountRestricted {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showNewCabalSheet = true
                    } label: {
                        Image(systemName: "plus")
                            .monacoToolbarIcon()
                            .frame(width: 44, height: 44)
                    }
                    .accessibilityLabel("New cabal")
                    .accessibilityIdentifier("cabals-new-button")
                }
            }
        }
        .sheet(
            isPresented: $showNewCabalSheet,
            onDismiss: {
                if let routeAfterSheet {
                    route = routeAfterSheet
                    self.routeAfterSheet = nil
                }
            }
        ) {
            NewCabalSheet(
                onCreate: {
                    routeAfterSheet = .create
                    showNewCabalSheet = false
                },
                onJoin: {
                    routeAfterSheet = .joinByCode
                    showNewCabalSheet = false
                }
            )
        }
        .navigationDestination(item: $route) { route in
            CabalsRouteDestination(
                auth: auth,
                route: route,
                actions: actions,
                onCreated: { created in
                    self.route = .cabal(id: created.id, name: created.name)
                }
            )
        }
        .refreshable {
            await refreshAll()
        }
        .task {
            if session.home == nil { await loadCabals() }
            await model.reload(hasCabals: !session.joinedCabals.isEmpty)
        }
        .onChange(of: searchText) { _, newValue in
            model.updateQuery(newValue)
        }
        .onChange(of: joinedIDs) { _, ids in
            // Joined, created, or left a cabal somewhere in the app.
            Task { await model.reload(hasCabals: !ids.isEmpty) }
        }
        .accessibilityIdentifier("cabals-root")
        .monacoFrameStats("Cabals")
    }

    private func refreshAll() async {
        // `session.refresh` loads the cabals list in a background task of its own,
        // so pull-to-refresh awaits that read directly: the spinner then ends when
        // the strip is actually up to date.
        async let profile: Void = session.refresh(auth: auth)
        async let cabals: Void = loadCabals()
        _ = await (profile, cabals)
        await model.reload(hasCabals: !session.joinedCabals.isEmpty)
    }

    private func loadCabals() async {
        isLoadingCabals = true
        defer { isLoadingCabals = false }
        await session.refreshHomeBoards(accessToken: auth.accessToken)
    }
}

#if DEBUG
#Preview {
    let session = AppSessionStore(apiClient: MonacoAPIClient())
    session.home = CabalsTabSampleData.home
    return NavigationStack {
        CabalsTabView(
            auth: PrivyAuthService(), dataSource: CabalsTabSampleData.DataSource(),
            actions: CabalsTabSampleData.Actions()
        )
        .environment(session)
        .monacoRootAppearance()
    }
}
#endif
