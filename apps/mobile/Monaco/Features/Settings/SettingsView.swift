import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum SettingsRow: CaseIterable, Identifiable {
    case notifications, editProfile, activity, withdraw, blockedPeople, advanced, signOut, deleteAccount

    static let accountRows: [SettingsRow] = [
        .editProfile, .activity, .withdraw, .blockedPeople, .advanced, .signOut,
    ]

    var id: Self { self }

    var title: String {
        switch self {
        case .notifications: "Notifications"
        case .editProfile: "Edit profile"
        case .activity: "Account activity"
        case .withdraw: "Withdraw"
        case .blockedPeople: "Blocked people"
        case .advanced: "Block explorers"
        case .signOut: "Sign out"
        case .deleteAccount: "Delete account"
        }
    }

    var subtitle: String? {
        switch self {
        case .notifications: nil
        case .editProfile: "Name, photo and optional bio"
        case .activity: "Transfers and receipts"
        case .withdraw: "Send cash to an external wallet"
        case .blockedPeople: "People you've blocked"
        case .advanced: "Choose where transaction links open"
        case .signOut: "Return to sign-in"
        case .deleteAccount: "Close your account and leave your cabals"
        }
    }

    var systemImage: String {
        switch self {
        case .notifications: "bell"
        case .editProfile: "person"
        case .activity: "doc.text"
        case .withdraw: "arrow.up.right"
        case .blockedPeople: "hand.raised"
        case .advanced: "link"
        case .signOut: "rectangle.portrait.and.arrow.right"
        case .deleteAccount: "trash"
        }
    }

    var identifier: String {
        switch self {
        case .notifications: "settings-notifications"
        case .editProfile: "settings-edit-profile"
        case .activity: "settings-activity"
        case .withdraw: "settings-withdraw"
        case .blockedPeople: "settings-blocked-people"
        case .advanced: "settings-advanced"
        case .signOut: "profileSignOutButton"
        case .deleteAccount: "settings-delete-account"
        }
    }
}

enum SettingsCopy {
    static let signOut = "Sign out"
    static let signOutTitle = "Sign out of Monaco?"
    static let signOutMessage =
        "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."
    static let stayLabel = "Stay signed in"
    static let saving = "Saving…"
    static let errorTitle = "Couldn't load this view"
    static let errorMessage =
        "Your existing information hasn't been changed. Try again when your connection is available."

    static func version(info: [String: Any]) -> String {
        let release = info["CFBundleShortVersionString"] as? String ?? ""
        let build = info["CFBundleVersion"] as? String ?? ""
        return "Monaco \(release) (\(build))"
    }
}

enum SettingsScreenState: Equatable {
    case ready, saving, error, signOut

    static func from(words: [String]) -> SettingsScreenState {
        if words.contains("saving") { return .saving }
        if words.contains("error") { return .error }
        if words.contains("signOut") { return .signOut }
        return .ready
    }
}

struct SettingsView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        SettingsList(
            authorization: LiveNotificationAuthorizing(), register: environment.registerForPush,
            signOut: { await environment.signOut() },
            preferences: { NotificationPreferencesModel(api: environment.api) },
            showError: { toasts.show($0) },
            open: { route in environment.navigator.open(route, in: environment.navigator.selectedTab) })
    }
}

struct SettingsList: View {
    let authorization: any NotificationAuthorizing
    let register: () -> Void
    let signOut: () async -> Void
    let showError: (APIError) -> Void
    let open: (any AppRoute) -> Void

    @Environment(\.openURL) private var openURL
    @Environment(\.scenePhase) private var scenePhase
    @State private var status: PushAuthorization?
    @State private var phase: SettingsScreenState
    @State private var isSigningOut = false
    @State private var preferences: NotificationPreferencesModel?
    @AccessibilityFocusState private var panelTitleFocused: Bool

    init(
        authorization: any NotificationAuthorizing,
        register: @escaping () -> Void,
        signOut: @escaping () async -> Void = {},
        state: SettingsScreenState = .ready,
        preferences: () -> NotificationPreferencesModel? = { nil },
        showError: @escaping (APIError) -> Void = { _ in },
        open: @escaping (any AppRoute) -> Void
    ) {
        self.authorization = authorization
        self.register = register
        self.signOut = signOut
        self.showError = showError
        self.open = open
        _phase = State(initialValue: state)
        _preferences = State(initialValue: preferences())
    }

    var body: some View {
        ScrollView {
            content
        }
        .monacoCanvas()
        .navigationTitle("Settings")
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            if phase == .saving {
                MonacoNotice(.info, message: SettingsCopy.saving)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.s)
                    .accessibilityIdentifier("settings-saving")
            }
        }
        .task {
            await readNotifications()
            await preferences?.load()
        }
        .onChange(of: scenePhase) { _, newPhase in
            guard newPhase == .active else { return }
            Task {
                await readNotifications()
                await preferences?.load()
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch phase {
        case .ready, .saving:
            sections
        case .error:
            EmptyState(
                title: SettingsCopy.errorTitle,
                message: SettingsCopy.errorMessage,
                actionTitle: "Try again",
                actionIdentifier: "settings-retry",
                systemImage: MonacoIcon.offline.symbol,
                action: { phase = .ready }
            )
        case .signOut:
            signOutPanel
        }
    }

    private var sections: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader("Notifications")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                notifications
            }
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader("Account")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ForEach(SettingsRow.accountRows) { row in
                        link(for: row)
                    }
                }
            }
            MonacoGroupedList {
                link(for: .deleteAccount)
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

    private var notifications: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoGroupedList {
                Button {
                    Task { await tapNotifications() }
                } label: {
                    MonacoRow(
                        title: SettingsRow.notifications.title,
                        subtitle: status.map { $0 == .authorized ? "On" : "Off" },
                        chevron: true,
                        isLast: preferences == nil,
                        leading: { SunkenGlyphMark(systemImage: SettingsRow.notifications.systemImage) }
                    )
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier(SettingsRow.notifications.identifier)
                if let preferences {
                    categoryRow(
                        "Decision requests", subtitle: "Votes and important state changes",
                        identifier: "settings-category-decisions", category: .decisionRequests,
                        preferences: preferences)
                    categoryRow(
                        "Replies and mentions", subtitle: "Updates directed at you",
                        identifier: "settings-category-replies", category: .repliesAndMentions,
                        preferences: preferences, isLast: true)
                }
            }
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
}

extension SettingsList {
    private func categoryRow(
        _ title: String, subtitle: String, identifier: String, category: NotificationCategory,
        preferences: NotificationPreferencesModel, isLast: Bool = false
    ) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Toggle(
                title,
                isOn: Binding(
                    get: { preferences.choices?[category] ?? true },
                    set: { enabled in Task { await save(category, enabled, in: preferences) } })
            )
            .labelsHidden()
            .tint(MonacoTheme.brandFill)
            .disabled(!preferences.canChange)
            .accessibilityIdentifier("\(identifier)-switch")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }

    private func save(
        _ category: NotificationCategory, _ enabled: Bool, in preferences: NotificationPreferencesModel
    ) async {
        if let error = await preferences.set(category, to: enabled) { showError(error) }
    }

    private var signOutPanel: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: 0) {
                Text(SettingsCopy.signOutTitle)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityFocused($panelTitleFocused)
                    .accessibilityIdentifier("profile-sign-out-title")
                Text(SettingsCopy.signOutMessage)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, MonacoTheme.Space.sm)
                VStack(spacing: MonacoTheme.Space.sm) {
                    Button {
                        guard !isSigningOut else { return }
                        isSigningOut = true
                        Task {
                            await signOut()
                            isSigningOut = false
                        }
                    } label: {
                        SubmitLabel(isWorking: isSigningOut, idle: SettingsCopy.signOut, working: "Signing out…")
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(isSigningOut)
                    .accessibilityIdentifier("profile-sign-out-confirm")
                    Button(SettingsCopy.stayLabel) {
                        phase = .ready
                    }
                    .buttonStyle(.monacoOutline)
                    .disabled(isSigningOut)
                    .accessibilityIdentifier("profile-sign-out-cancel")
                }
                .padding(.top, MonacoTheme.Space.l)
            }
            .padding(MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .onAppear { panelTitleFocused = true }
    }

    @ViewBuilder private func link(for row: SettingsRow) -> some View {
        let isLast = row == SettingsRow.accountRows.last || row == .deleteAccount
        switch row {
        case .notifications:
            EmptyView()
        case .editProfile:
            Button {
                open(EditProfileRoute())
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .activity:
            Button {
                open(AccountActivityRoute())
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .withdraw:
            Button {
                open(WithdrawRoute())
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .blockedPeople:
            Button {
                open(BlockedPeopleRoute())
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .advanced:
            NavigationLink {
                AdvancedSettingsView()
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .signOut:
            Button {
                phase = .signOut
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .disabled(isSigningOut)
            .accessibilityIdentifier(row.identifier)
        case .deleteAccount:
            Button {
                open(DeleteAccountRoute())
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        }
    }

    private func label(for row: SettingsRow, isLast: Bool) -> some View {
        MonacoRow(
            title: row.title,
            titleColor: row == .deleteAccount ? MonacoTheme.destructive : MonacoTheme.primaryText,
            subtitle: row.subtitle,
            chevron: row != .signOut && row != .deleteAccount,
            isLast: isLast,
            leading: { SunkenGlyphMark(systemImage: row.systemImage) }
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
        let words = arguments.dropFirst(flag + 1).prefix(2)
        let state = SettingsScreenState.from(words: Array(words))
        return AnyView(
            SettingsHarnessScreen(
                authorized: words.contains("on"), state: state, showsCategorySwitches: words.contains("categories")))
    }
}

private struct SettingsHarnessScreen: View {
    let authorized: Bool
    let state: SettingsScreenState
    let showsCategorySwitches: Bool
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            SettingsList(
                authorization: FixedAuthorization(current: authorized ? .authorized : .denied), register: {},
                state: state, preferences: { showsCategorySwitches ? .preview() : nil }
            ) {
                path.append(AnyAppRoute($0))
            }
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}
#endif
