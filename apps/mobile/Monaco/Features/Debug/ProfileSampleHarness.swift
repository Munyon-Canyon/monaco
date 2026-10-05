#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

/// Debug-only: renders Profile from canned `AppSessionStore` data so QA can screenshot
/// each state without Privy or a backend. Launch with
/// `-MonacoProfileSample <placeholder|photo|facePicker|validation|saveFailure|saveSuccess|cabals|empty|loading|error>`.
/// `cabals` and `empty` open scrolled to the bottom so the cabal list is on screen.
enum ProfileSampleScenario: String, CaseIterable {
    case placeholder
    case photo
    /// The face sheet open over the placeholder, so the grid and its ring can be screenshotted.
    case facePicker
    case validation
    /// Edit profile open on a valid new name. There is no session here, so tapping Save is
    /// a rejected save — which is how the failure is meant to be readable inside the sheet.
    case saveFailure
    /// The same sheet with a store that accepts the save, so the other half of the fix — the
    /// sheet closing first and the toast landing on the uncovered screen — can be seen.
    case saveSuccess
    case cabals
    case empty
    case loading
    case error

    static func matching(_ arguments: [String]) -> ProfileSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoProfileSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return ProfileSampleScenario(rawValue: arguments[flag + 1])
    }
}

struct ProfileSampleHarness: View {
    let scenario: ProfileSampleScenario
    @ObservedObject var auth: PrivyAuthService
    @State private var environment: AppEnvironment
    @State private var refresh = ScreenRefresh()

    init(scenario: ProfileSampleScenario, auth: PrivyAuthService) {
        self.scenario = scenario
        self.auth = auth
        let environment = AppEnvironment(
            auth: auth,
            hints: SilentHints(),
            sessionStore: AppSessionStore(apiClient: MonacoAPIClient()),
            isAuthenticated: { true },
            endAuthSession: {}
        )
        Self.fill(environment.sessionStore, for: scenario)
        _environment = State(initialValue: environment)
    }

    var body: some View {
        NavigationStack {
            ProfileScreen()
                .refreshable { await refresh.run() }
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .defaultScrollAnchor(scenario == .cabals || scenario == .empty ? .bottom : .top)
        .environment(environment)
        .environment(environment.sessionStore)
        .environment(refresh)
        .environment(\.profileHeaderPresets, presets)
    }

    private var presets: ProfileHeaderPresets {
        ProfileHeaderPresets(
            nameDraft: Self.nameDraft(for: scenario),
            showsEditProfile: scenario == .validation || scenario == .saveFailure || scenario == .saveSuccess,
            showsFacePicker: scenario == .facePicker,
            saveName: saveNameOverride
        )
    }

    private var saveNameOverride: (any DisplayNameSaving)? {
        scenario == .saveSuccess ? AcceptingNameStore(session: environment.sessionStore) : nil
    }

    private static func nameDraft(for scenario: ProfileSampleScenario) -> String? {
        switch scenario {
        // Over the 32-character limit: the field shows the broken rule and Save stays off.
        case .validation: return "Logan Norman of the Weekend Investors"
        // Valid and different from the saved name, so Save is live and can be rejected.
        case .saveFailure: return "Logan N"
        // Valid and different, and this scenario's store accepts it.
        case .saveSuccess: return "Logan N"
        default: return nil
        }
    }

    private static func fill(_ session: AppSessionStore, for scenario: ProfileSampleScenario) {
        session.isLoading = false
        switch scenario {
        case .loading:
            session.isLoading = true
            return
        case .error:
            session.errorMessage = "Could not connect to Monaco."
            return
        default:
            break
        }

        session.profile = sampleProfile(
            userID: "sample-user",
            displayName: "Logan Norman",
            photoURL: scenario == .photo || scenario == .cabals ? samplePhotoURL() : nil,
            createdAt: ISO8601DateFormatter().date(from: "2026-09-01T14:30:00Z")
        )

        let joined = scenario != .empty
        session.home = HomeViewDTO(
            groups: joined
                ? [
                    HomeGroupBoardRowDTO(
                        groupId: "g1", name: "Weekend investors", potValueUsd: "548.20", percentReturn: "0.124",
                        dollarPnl: "+48.20", isJoined: true),
                    HomeGroupBoardRowDTO(
                        groupId: "g2", name: "Semis or bust", potValueUsd: "2310.75", percentReturn: "-0.031",
                        dollarPnl: "-73.90", isJoined: true),
                    HomeGroupBoardRowDTO(
                        groupId: "g3", name: "Index huggers", potValueUsd: "120.00", percentReturn: nil,
                        dollarPnl: "+0.00", isJoined: true),
                ] : [],
            people: []
        )
        session.dashboard = HomeDashboardDTO(
            netWorthUsd: joined ? "711.55" : "0.00",
            netWorthDollarPnl: joined ? "+12.40" : "+0.00",
            netWorthPercentReturn: joined ? "0.018" : nil,
            myGroups: joined
                ? [
                    HomeMyGroupRowDTO(
                        groupId: "g1", name: "Weekend investors", equityUsd: "311.50", slicePercent: "0.568",
                        dollarPnl: "+27.40", percentReturn: "0.096"),
                    HomeMyGroupRowDTO(
                        groupId: "g2", name: "Semis or bust", equityUsd: "400.05", slicePercent: "0.173",
                        dollarPnl: "-15.00", percentReturn: "-0.036"),
                    // Flat P&L: the row must still show the $120.00 the member has in it.
                    HomeMyGroupRowDTO(
                        groupId: "g3", name: "Index huggers", equityUsd: "120.00", slicePercent: "1.0",
                        dollarPnl: "+0.00", percentReturn: nil),
                ] : [],
            pnlSeries1H: [],
            leaderboard: HomeLeaderboardSectionDTO(range: "ALL", people: [])
        )
    }

    static func sampleProfile(userID: String, displayName: String, photoURL: URL?, createdAt: Date? = nil)
        -> SessionProfile
    {
        var profile = SessionProfile(Components.Schemas.Me.sample)
        profile.userID = userID
        profile.displayName = displayName
        profile.photoURL = photoURL
        if let createdAt { profile.createdAt = createdAt }
        return profile
    }

    /// Writes a generated landscape to tmp so AsyncImage loads it from a file URL.
    /// A generated portrait written to a temp file, so avatars show a photo without the network.
    static func samplePhotoURL() -> URL? {
        let size = CGSize(width: 256, height: 256)
        let image = UIGraphicsImageRenderer(size: size).image { context in
            let cg = context.cgContext
            let sky =
                [
                    UIColor(red: 0.98, green: 0.72, blue: 0.45, alpha: 1).cgColor,
                    UIColor(red: 0.85, green: 0.42, blue: 0.38, alpha: 1).cgColor,
                ] as CFArray
            if let gradient = CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(), colors: sky, locations: [0, 1]) {
                cg.drawLinearGradient(
                    gradient, start: .zero, end: CGPoint(x: 0, y: 170), options: [.drawsAfterEndLocation])
            }
            UIColor(red: 1.0, green: 0.93, blue: 0.7, alpha: 1).setFill()
            cg.fillEllipse(in: CGRect(x: 150, y: 70, width: 64, height: 64))
            UIColor(red: 0.29, green: 0.36, blue: 0.33, alpha: 1).setFill()
            cg.fillEllipse(in: CGRect(x: -60, y: 150, width: 260, height: 200))
            UIColor(red: 0.18, green: 0.25, blue: 0.23, alpha: 1).setFill()
            cg.fillEllipse(in: CGRect(x: 90, y: 175, width: 240, height: 180))
        }
        guard let data = image.pngData() else { return nil }
        let url = FileManager.default.temporaryDirectory.appending(path: "monaco-sample-avatar.png")
        do {
            try data.write(to: url, options: .atomic)
            return url
        } catch {
            return nil
        }
    }
}

private nonisolated struct SilentHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
    func start() async {}
    func stop() async {}
}

/// A store that accepts the save, standing in for the profile endpoint. Writes the name back
/// to the session, so the screen behind the sheet really does show it afterwards.
private struct AcceptingNameStore: DisplayNameSaving {
    let session: AppSessionStore

    func saveDisplayName(_ draft: String) async -> ProfileSaveOutcome {
        guard case .success(let normalized) = DisplayNameRules.normalize(draft) else {
            return .failed("That name can't be used.")
        }
        guard let current = session.profile else { return .failed("Your profile is still loading.") }
        guard normalized != current.displayName else { return .unchanged }
        session.profile = current.withDisplayName(normalized)
        return .saved
    }
}

final class ProfileSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProfileSampleScenario.matching(arguments) else { return nil }
        return AnyView(ProfileSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
