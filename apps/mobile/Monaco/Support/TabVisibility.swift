import SwiftUI

private struct SelectedMainTabKey: EnvironmentKey {
    static let defaultValue: MainTab? = nil
}

private struct HostMainTabKey: EnvironmentKey {
    static let defaultValue: MainTab? = nil
}

extension EnvironmentValues {
    var selectedMainTab: MainTab? {
        get { self[SelectedMainTabKey.self] }
        set { self[SelectedMainTabKey.self] = newValue }
    }

    var hostMainTab: MainTab? {
        get { self[HostMainTabKey.self] }
        set { self[HostMainTabKey.self] = newValue }
    }
}

extension View {
    func onScreenVisibilityChange(_ action: @escaping (Bool) -> Void) -> some View {
        modifier(ScreenVisibilityChange(action: action))
    }
}

private struct ScreenVisibilityChange: ViewModifier {
    let action: (Bool) -> Void

    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.selectedMainTab) private var selectedMainTab
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var appeared = false

    func body(content: Content) -> some View {
        content
            .onAppear {
                appeared = true
                action(isOnScreen)
            }
            .onDisappear {
                appeared = false
                action(false)
            }
            .onChange(of: scenePhase) { _, _ in
                action(isOnScreen)
            }
            .onChange(of: selectedMainTab) { _, _ in
                action(isOnScreen)
            }
            .onChange(of: hostMainTab) { _, _ in
                action(isOnScreen)
            }
    }

    private var isOnScreen: Bool {
        guard appeared, scenePhase == .active else { return false }
        guard let hostMainTab, let selectedMainTab else { return true }
        return hostMainTab == selectedMainTab
    }
}
