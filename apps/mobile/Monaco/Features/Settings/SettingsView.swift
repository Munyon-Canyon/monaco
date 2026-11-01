import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum SettingsRow: CaseIterable, Identifiable {
    case notifications, editProfile, findFriends, inviteFriends, viewAsOthers, activity, withdraw, blockedPeople
    case advanced, contactSupport, signOutEverywhere, signOut, deleteAccount

    static let accountRows: [SettingsRow] = [
        .editProfile, .findFriends, .inviteFriends, .viewAsOthers, .activity, .withdraw, .blockedPeople, .advanced,
        .contactSupport, .signOutEverywhere, .signOut,
    ]

    var id: Self { self }

    var title: String {
        switch self {
        case .notifications: "Notifications"
        case .editProfile: "Edit profile"
        case .findFriends: "Find friends"
        case .inviteFriends: "Invite friends"
        case .viewAsOthers: "View as others"
        case .activity: "Account activity"
        case .withdraw: "Withdraw"
        case .blockedPeople: "Blocked people"
        case .advanced: "Block explorers"
        case .contactSupport: "Contact support"
        case .signOutEverywhere: "Sign out of all devices"
        case .signOut: "Sign out"
        case .deleteAccount: "Delete account"
        }
    }

    var subtitle: String? {
        switch self {
        case .notifications: nil
        case .editProfile: "Name, photo and optional bio"
        case .findFriends: "Search people and contacts"
        case .inviteFriends: "Share your invite link"
        case .viewAsOthers: "See your profile as others do"
        case .activity: "Transfers and receipts"
        case .withdraw: "Send cash to an external wallet"
        case .blockedPeople: "People you've blocked"
        case .advanced: "Open transactions on a block explorer"
        case .contactSupport: SettingsCopy.supportEmail
        case .signOutEverywhere: "End every session, including a lost phone"
        case .signOut: "Return to sign-in"
        case .deleteAccount: "Cash out and withdraw first, then close your account"
        }
    }

    var systemImage: String {
        switch self {
        case .notifications: "bell"
        case .editProfile: "person"
        case .findFriends: "person.2"
        case .inviteFriends: "square.and.arrow.up"
        case .viewAsOthers: "eye"
        case .activity: "doc.text"
        case .withdraw: "arrow.up.right"
        case .blockedPeople: "hand.raised"
        case .advanced: "link"
        case .contactSupport: "envelope"
        case .signOutEverywhere: "iphone.slash"
        case .signOut: "rectangle.portrait.and.arrow.right"
        case .deleteAccount: "trash"
        }
    }

    var identifier: String {
        switch self {
        case .notifications: "settings-notifications"
        case .editProfile: "settings-edit-profile"
        case .findFriends: "profile-find-friends"
        case .inviteFriends: "profile-invite-row"
        case .viewAsOthers: "profile-view-as-others"
        case .activity: "settings-activity"
        case .withdraw: "settings-withdraw"
        case .blockedPeople: "settings-blocked-people"
        case .advanced: "settings-advanced"
        case .contactSupport: "settings-contact-support"
        case .signOutEverywhere: "settings-sign-out-everywhere"
        case .signOut: "profileSignOutButton"
        case .deleteAccount: "settings-delete-account"
        }
    }
}

enum SettingsCopy {
    static let supportEmail = "support@monacolabs.xyz"
    static let supportURL = URL(string: "mailto:\(supportEmail)?subject=Monaco%20support") ?? URL(fileURLWithPath: "/")
    static let signOut = "Sign out"
    static let signOutTitle = "Sign out of Monaco?"
    static let signOutMessage =
        "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."
    static let signOutEverywhere = "Sign out of all devices"
    static let signOutEverywhereTitle = "Sign out of all devices?"
    static let signOutEverywhereMessage =
        "This ends every session on your account, including a lost phone. "
        + "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."
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
    case ready, saving, error, signOut, signOutEverywhere

    static func from(words: [String]) -> SettingsScreenState {
        if words.contains("saving") { return .saving }
        if words.contains("error") { return .error }
        if words.contains("signOutEverywhere") { return .signOutEverywhere }
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
            signOutEverywhere: { await environment.signOutEverywhere() },
            preferences: { NotificationPreferencesModel(api: environment.api) },
            showError: { toasts.show($0) }, viewerID: environment.viewer?.userID,
            open: { route in environment.navigator.open(route, in: environment.navigator.selectedTab) })
    }
}

struct SettingsList: View {
    let authorization: any NotificationAuthorizing
    let register: () -> Void
    let signOut: () async -> Void
    let signOutEverywhere: () async -> Bool
    let showError: (APIError) -> Void
    let viewerID: String?
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
        signOutEverywhere: @escaping () async -> Bool = { true },
        state: SettingsScreenState = .ready,
        preferences: () -> NotificationPreferencesModel? = { nil },
        showError: @escaping (APIError) -> Void = { _ in },
        viewerID: String? = nil,
        open: @escaping (any AppRoute) -> Void
    ) {
        self.authorization = authorization
        self.register = register
        self.signOut = signOut
        self.signOutEverywhere = signOutEverywhere
        self.showError = showError
        self.viewerID = viewerID
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
        case .signOutEverywhere:
            signOutEverywherePanel
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
                    ForEach(SettingsRow.accountRows.filter { $0 != .viewAsOthers || viewerID != nil }) { row in
                        link(for: row)
                    }
                }
            }
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader("Privacy")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    AnalyticsToggleRow()
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
                    get: { preferences.choices?[category] ?? false },
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
        confirmationPanel(
            title: SettingsCopy.signOutTitle, message: SettingsCopy.signOutMessage, confirm: SettingsCopy.signOut,
            identifier: "profile-sign-out"
        ) {
            await signOut()
            return true
        }
    }

    private var signOutEverywherePanel: some View {
        confirmationPanel(
            title: SettingsCopy.signOutEverywhereTitle, message: SettingsCopy.signOutEverywhereMessage,
            confirm: SettingsCopy.signOutEverywhere, identifier: "profile-sign-out-everywhere"
        ) {
            let ended = await signOutEverywhere()
            if !ended { showError(.transport(URLError(.notConnectedToInternet))) }
            return ended
        }
    }

    private func confirmationPanel(
        title: String, message: String, confirm: String, identifier: String,
        action: @escaping () async -> Bool
    ) -> some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: 0) {
                Text(title)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityFocused($panelTitleFocused)
                    .accessibilityIdentifier("\(identifier)-title")
                Text(message)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, MonacoTheme.Space.sm)
                VStack(spacing: MonacoTheme.Space.sm) {
                    Button {
                        guard !isSigningOut else { return }
                        isSigningOut = true
                        Task {
                            let ended = await action()
                            isSigningOut = false
                            if !ended { phase = .ready }
                        }
                    } label: {
                        SubmitLabel(isWorking: isSigningOut, idle: confirm, working: "Signing out…")
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(isSigningOut)
                    .accessibilityIdentifier("\(identifier)-confirm")
                    Button(SettingsCopy.stayLabel) {
                        phase = .ready
                    }
                    .buttonStyle(.monacoOutline)
                    .disabled(isSigningOut)
                    .accessibilityIdentifier("\(identifier)-cancel")
                }
                .padding(.top, MonacoTheme.Space.l)
            }
            .padding(MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .onAppear { panelTitleFocused = true }
    }

    private func route(for row: SettingsRow) -> (any AppRoute)? {
        switch row {
        case .editProfile: EditProfileRoute()
        case .findFriends: FriendsRoute()
        case .inviteFriends: InviteRoute()
        case .viewAsOthers: viewerID.map { UserProfileRoute(userID: $0) }
        case .activity: AccountActivityRoute()
        case .withdraw: WithdrawRoute()
        case .blockedPeople: BlockedPeopleRoute()
        case .deleteAccount: DeleteAccountRoute()
        case .notifications, .advanced, .contactSupport, .signOutEverywhere, .signOut: nil
        }
    }

    @ViewBuilder private func link(for row: SettingsRow) -> some View {
        let isLast = row == SettingsRow.accountRows.last || row == .deleteAccount
        switch row {
        case .notifications:
            EmptyView()
        case .advanced:
            NavigationLink {
                AdvancedSettingsView()
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .contactSupport:
            Button {
                openURL(SettingsCopy.supportURL)
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier(row.identifier)
        case .signOutEverywhere:
            Button {
                phase = .signOutEverywhere
            } label: {
                label(for: row, isLast: isLast)
            }
            .buttonStyle(.monacoRow)
            .disabled(isSigningOut)
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
        default:
            Button {
                if let route = route(for: row) { open(route) }
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
            chevron: row != .signOut && row != .signOutEverywhere && row != .deleteAccount,
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
                state: state, preferences: { showsCategorySwitches ? .preview() : nil }, viewerID: "sample-me"
            ) {
                path.append(AnyAppRoute($0))
            }
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}
#endif
