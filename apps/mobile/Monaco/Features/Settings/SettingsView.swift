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
        SettingsList { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
    }
}

struct SettingsList: View {
    let open: (any AppRoute) -> Void

    @Environment(\.openURL) private var openURL

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
    }

    @ViewBuilder private func link(for row: SettingsRow) -> some View {
        switch row {
        case .notifications:
            Button {
                guard let url = URL(string: UIApplication.openNotificationSettingsURLString) else { return }
                openURL(url)
            } label: {
                label(for: row)
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
            chevron: row != .notifications && row != .deleteAccount,
            isLast: row == SettingsRow.allCases.last,
            leading: { StockMark(systemImage: row.systemImage, size: 40) }
        )
    }
}

#Preview {
    NavigationStack {
        SettingsList { _ in }
    }
}
