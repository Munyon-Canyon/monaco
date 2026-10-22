import MonacoCore
import SwiftUI

/// Post-auth frame. Tab chrome only — screens live in their feature folders.
struct MainTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.accountRestricted) private var accountRestricted

    var body: some View {
        @Bindable var navigator = environment.navigator
        @Bindable var pushPrePrompt = environment.pushPrePrompt
        TabView(selection: Binding(get: { navigator.selectedTab }, set: { navigator.select($0) })) {
            ForEach(MainTab.allCases) { tab in
                NavigationStack(path: navigator.binding(for: tab)) {
                    tab.root
                        .monacoCanvas()
                        .safeAreaInset(edge: .top, spacing: 0) {
                            if accountRestricted { AccountUnderReviewNotice() }
                        }
                        .hardBottomScrollEdge()
                        .analyticsScreen(String(describing: tab))
                        .tabRouteDestinations()
                }
                .monacoToastCenter(toasts, isEnabled: tab == navigator.selectedTab)
                .tabItem {
                    MainTabLabel(tab: tab, isSelected: tab == navigator.selectedTab)
                        .accessibilityIdentifier(tab.accessibilityIdentifier)
                }
                .tag(tab)
                .environment(\.hostMainTab, tab)
            }
        }
        .tint(MonacoTheme.brand)
        .environment(\.depositNavigator, navigator)
        .modifier(AccountBalanceHost())
        .preference(key: TabToastHostKey.self, value: true)
        // Each stack knows its tab (`hostMainTab`) and which one is showing, so screens in a tab
        // the member switched away from stop polling. See `pollWhileVisible`.
        .environment(\.selectedMainTab, navigator.selectedTab)
        .onChange(of: navigator.selectedTab) { _, _ in
            Haptics.selection()
        }
        .sheet(item: $navigator.deposit) { request in
            DepositView(prefillMicros: request.prefillMicros, cabalID: request.cabalID)
                .analyticsScreen("deposit")
        }
        .sheet(isPresented: $pushPrePrompt.isPresented, onDismiss: pushPrePrompt.notNow) {
            PushPrePromptSheet(prompt: pushPrePrompt)
        }
    }
}

struct MainTabLabel: View {
    let tab: MainTab
    let isSelected: Bool

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        if let icon = MonacoIcon(systemName: tab.systemImage),
            let image = icon.tabImage(selected: isSelected, scale: displayScale)
        {
            Label {
                Text(tab.title)
            } icon: {
                Image(uiImage: image)
            }
        } else {
            Label(tab.title, systemImage: tab.systemImage)
                .environment(\.symbolVariants, isSelected ? .fill : .none)
        }
    }
}

struct AccountBalanceHost: ViewModifier {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.scenePhase) private var scenePhase

    func body(content: Content) -> some View {
        content
            .task {
                await environment.balance.load()
                await environment.balance.observe()
            }
            .onChange(of: scenePhase) { _, phase in
                environment.balance.setVisible(phase == .active)
            }
            .onChange(of: environment.balance.balance) { previous, current in
                guard let current, let change = BalanceChange.detect(previous: previous, current: current) else {
                    return
                }
                environment.cardDeposit.balanceChanged(change)
                toasts.show(success: change.message)
            }
            .onChange(of: environment.balance.failureTick) { _, _ in
                guard environment.balance.balance != nil, let error = environment.balance.lastError else { return }
                toasts.current = MonacoToast(message: BalanceSource.message(for: error))
            }
    }
}

extension View {
    @ViewBuilder
    fileprivate func hardBottomScrollEdge() -> some View {
        if #available(iOS 26, *) {
            scrollEdgeEffectStyle(.hard, for: .bottom)
        } else {
            self
        }
    }
}

extension View {
    func tabRouteDestinations() -> some View {
        navigationDestination(for: AnyAppRoute.self) { route in
            route.destination()
                .monacoCanvas()
                .navigationBarTitleDisplayMode(.inline)
                .toolbarBackground(MonacoTheme.canvas, for: .navigationBar)
                .toolbarBackground(.visible, for: .navigationBar)
        }
    }
}
