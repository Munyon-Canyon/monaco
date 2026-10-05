import MonacoCore
import SwiftUI

/// The five product tabs. Account actions (withdraw, advanced, sign out) live on Profile.
/// Post-auth frame. Tab chrome only — screens live in their feature folders.
struct MainTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        @Bindable var navigator = environment.navigator
        @Bindable var pushPrePrompt = environment.pushPrePrompt
        TabView(selection: $navigator.selectedTab) {
            ForEach(MainTab.allCases) { tab in
                NavigationStack(path: navigator.binding(for: tab)) {
                    tab.root
                        .navigationDestination(for: AnyAppRoute.self) { route in
                            route.destination()
                        }
                }
                .tabItem {
                    Label(tab.title, systemImage: tab.systemImage)
                        .accessibilityIdentifier(tab.accessibilityIdentifier)
                }
                .tag(tab)
                .environment(\.hostMainTab, tab)
            }
        }
        .tint(MonacoTheme.ink)
        // Each stack knows its tab (`hostMainTab`) and which one is showing, so screens in a tab
        // the member switched away from stop polling. See `pollWhileVisible`.
        .environment(\.selectedMainTab, navigator.selectedTab)
        .onChange(of: navigator.selectedTab) { _, _ in
            Haptics.selection()
        }
        .onChange(of: environment.cashOuts.notice) { _, notice in
            guard let notice else { return }
            toasts.current = MonacoToast(message: notice.message, isSuccess: notice.isSuccess)
        }
        .sheet(isPresented: $pushPrePrompt.isPresented, onDismiss: pushPrePrompt.notNow) {
            PushPrePromptSheet(prompt: pushPrePrompt)
        }
    }
}
