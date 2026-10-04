import MonacoCore
import SwiftUI
import UIKit

enum SettingsRow: CaseIterable, Identifiable {
    case notifications, activity, withdraw, advanced, deleteAccount

    var id: Self { self }

    var title: String {
        switch self {
        case .notifications: "Notifications"
        case .activity: "Activity"
        case .withdraw: "Withdraw"
        case .advanced: "Advanced"
        case .deleteAccount: "Delete account"
        }
    }

    var subtitle: String? {
        self == .advanced ? "Block explorers" : nil
    }

    var systemImage: String {
        switch self {
        case .notifications: "bell"
        case .activity: "clock.arrow.circlepath"
        case .withdraw: "arrow.down.left"
        case .advanced: "link"
        case .deleteAccount: "trash"
        }
    }

    var identifier: String {
        switch self {
        case .notifications: "settings-notifications"
        case .activity: "settings-activity"
        case .withdraw: "settings-withdraw"
        case .advanced: "settings-advanced"
        case .deleteAccount: "settings-delete-account"
        }
    }
}

enum SettingsCopy {
    static func version(info: [String: Any]) -> String {
        let release = info["CFBundleShortVersionString"] as? String ?? ""
        let build = info["CFBundleVersion"] as? String ?? ""
        return "Monaco \(release) (\(build))"
    }
}

struct SettingsView: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        SettingsList(authorization: LiveNotificationAuthorizing(), register: environment.registerForPush) { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
    }
}

struct SettingsList: View {
    let authorization: any NotificationAuthorizing
    let register: () -> Void
    let open: (any AppRoute) -> Void

    @Environment(\.openURL) private var openURL
    @Environment(\.scenePhase) private var scenePhase
    @State private var status: PushAuthorization?

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.m) {
                MonacoGroupedList {
                    ForEach(SettingsRow.allCases) { row in
                        link(for: row)
                            .buttonStyle(.monacoRow)
                            .accessibilityIdentifier(row.identifier)
                    }
                }
                Text(SettingsCopy.version(info: Bundle.main.infoDictionary ?? [:]))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("settings-version")
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Settings")
        .navigationBarTitleDisplayMode(.inline)
        .task { await readNotifications() }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active else { return }
            Task { await readNotifications() }
        }
    }

    private func readNotifications() async {
        status = await authorization.status()
    }

    private func tapNotifications() async {
        switch await authorization.status() {
        case .notDetermined:
            if await authorization.request() { register() }
            await readNotifications()
        case .denied, .authorized:
            guard let url = URL(string: UIApplication.openNotificationSettingsURLString) else { return }
            openURL(url)
        }
    }

    @ViewBuilder private func link(for row: SettingsRow) -> some View {
        switch row {
        case .notifications:
            Button {
                Task { await tapNotifications() }
            } label: {
                MonacoRow(
                    title: row.title,
                    leading: { StockMark(systemImage: row.systemImage, size: 40) },
                    trailing: {
                        Text(status.map { $0 == .authorized ? "On" : "Off" } ?? "")
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.muted)
                    }
                )
            }
        case .activity:
            Button {
                open(AccountActivityRoute())
            } label: {
                label(for: row)
            }
        case .withdraw:
            Button {
                open(WithdrawRoute())
            } label: {
                label(for: row)
            }
        case .advanced:
            NavigationLink {
                AdvancedSettingsView()
            } label: {
                label(for: row)
            }
        case .deleteAccount:
            Button {
                open(DeleteAccountRoute())
            } label: {
                label(for: row)
            }
        }
    }

    private func label(for row: SettingsRow) -> some View {
        MonacoRow(
            title: row.title,
            titleColor: row == .deleteAccount ? MonacoTheme.destructive : MonacoTheme.ink,
            subtitle: row.subtitle,
            chevron: row != .deleteAccount,
            isLast: row == SettingsRow.allCases.last,
            leading: { StockMark(systemImage: row.systemImage, size: 40) }
        )
    }
}

#if DEBUG
private struct FixedAuthorization: NotificationAuthorizing {
    let current: PushAuthorization

    func status() async -> PushAuthorization { current }

    func request() async -> Bool { current == .authorized }
}

#Preview {
    NavigationStack {
        SettingsList(authorization: FixedAuthorization(current: .authorized), register: {}) { _ in }
    }
}

final class SettingsSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-settingsHarness") else { return nil }
        let authorized = arguments.indices.contains(flag + 1) && arguments[flag + 1] == "on"
        return AnyView(SettingsHarnessScreen(authorized: authorized))
    }
}

private struct SettingsHarnessScreen: View {
    let authorized: Bool
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            SettingsList(authorization: FixedAuthorization(current: authorized ? .authorized : .denied), register: {}) {
                path.append(AnyAppRoute($0))
            }
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}
#endif
