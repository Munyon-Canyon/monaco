import MonacoAPI
import MonacoCore
import SwiftUI

/// Cabals tab shell.
struct CabalsTabView: View {
    @ObservedObject var auth: PrivyAuthService
    @Environment(AppSessionStore.self) private var session
    @Environment(\.accountRestricted) private var accountRestricted
    @State private var showNewCabalSheet = false
    /// The pushed screen, if any. One item for the whole tab: see `CabalsRoute`.
    @State private var route: CabalsRoute?
    /// Chosen in the "New cabal" sheet, pushed once the sheet is gone. Pushing
    /// in the same turn as the dismissal makes the stack change mid-transition.
    @State private var routeAfterSheet: CabalsRoute?
    private let actions: CabalsActionSource

    init(auth: PrivyAuthService, actions: CabalsActionSource) {
        self.auth = auth
        self.actions = actions
    }

    var body: some View {
        MonacoScreen {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {}
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
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-root")
        .monacoFrameStats("Cabals")
    }

    private func refreshAll() async {
        await session.refresh(auth: auth)
    }
}

#if DEBUG
#Preview {
    let session = AppSessionStore()
    return NavigationStack {
        CabalsTabView(auth: PrivyAuthService(), actions: CabalsTabSampleData.Actions())
            .environment(session)
            .monacoRootAppearance()
    }
}
#endif
