import MonacoAPI
import MonacoCore
import SwiftUI

enum UserProfileTabsSlot: UserProfileSection {
    static let isLive = true
    static let defaultSelection = ProfileTabsSelection.activity

    static func isVisible(phase: UserProfileModel.Phase?, blockedByMe: Bool) -> Bool {
        phase == .loaded && !blockedByMe
    }

    static func body(for context: UserProfileContext) -> some View {
        UserProfileTabsLive(userID: context.userID)
    }
}

struct UserProfileTabsContent: View {
    let cabals: LoadState<[UserCabalRow]>
    let activity: LoadState<[MemberActivityRow]>
    let cabalCount: Int?
    let hasMoreActivity: Bool
    @Binding var selection: ProfileTabsSelection
    var retryCabals: () -> Void = {}
    var retryActivity: () -> Void = {}
    var loadMore: () -> Void = {}

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoUnderlineTabs(
                ProfileTabsSelection.allCases, selection: $selection,
                count: { $0 == .cabals ? cabalCount : nil },
                identifier: { $0 == .cabals ? "user-profile-tab-cabals" : "user-profile-tab-activity" },
                label: { $0 == .cabals ? "Cabals" : "Activity" }
            )
            .accessibilityIdentifier("user-profile-tabs")
            switch selection {
            case .cabals: cabalsBody
            case .activity: activityBody
            }
        }
    }

    @ViewBuilder private var cabalsBody: some View {
        switch cabals {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3).accessibilityIdentifier("user-profile-cabals-loading")
        case .failed:
            failure(identifier: "user-profile-cabals-error", retry: retryCabals)
        case .loaded(let rows) where rows.isEmpty:
            EmptyState(title: "Not in a cabal yet.")
                .padding(.vertical, MonacoTheme.Space.l)
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("user-profile-cabals-empty")
        case .loaded(let rows):
            MonacoGroupedList {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    ProfileCabalRowView(userCabal: row, isLast: index == rows.count - 1)
                }
            }
        }
    }

    @ViewBuilder private var activityBody: some View {
        switch activity {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .none).accessibilityIdentifier("user-profile-activity-loading")
        case .failed:
            failure(identifier: "user-profile-activity-error", retry: retryActivity)
        case .loaded(let rows) where rows.isEmpty:
            EmptyState(title: "Nothing yet.")
                .padding(.vertical, MonacoTheme.Space.l)
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("user-profile-activity-empty")
        case .loaded(let rows):
            MonacoGroupedList {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    ProfileActivityRowView(
                        row: row, index: index, isLast: index == rows.count - 1, identifierPrefix: "user-profile"
                    )
                    .onAppear { if hasMoreActivity, index == rows.count - 1 { loadMore() } }
                }
            }
        }
    }

    private func failure(identifier: String, retry: @escaping () -> Void) -> some View {
        EmptyState(
            title: "Couldn't load this", actionTitle: "Try again", actionIdentifier: identifier, action: retry
        )
        .padding(.vertical, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity)
    }
}

private struct UserProfileTabsLive: View {
    let userID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(UserProfileModel.self) private var profile: UserProfileModel?
    @State private var cabals: UserCabalsModel?
    @State private var activity: MemberActivityModel?
    @State private var selection = UserProfileTabsSlot.defaultSelection

    private var cabalCount: Int? {
        if case .loaded(let rows) = cabals?.state { return rows.count }
        return profile?.cabalCount
    }

    var body: some View {
        if UserProfileTabsSlot.isVisible(phase: profile?.phase, blockedByMe: profile?.blockedByMe ?? false) {
            UserProfileTabsContent(
                cabals: cabals?.state ?? .loading, activity: activity?.state ?? .loading, cabalCount: cabalCount,
                hasMoreActivity: activity?.hasMore ?? false, selection: $selection,
                retryCabals: { Task { await cabals?.load() } },
                retryActivity: { Task { await activity?.load() } },
                loadMore: { Task { await activity?.loadMore() } }
            )
            .task(id: userID) {
                let cabalsModel = UserCabalsModel(userID: userID, api: environment.api)
                let activityModel = MemberActivityModel(userID: userID, api: environment.api)
                cabals = cabalsModel
                activity = activityModel
                refresh?.register("user-profile-tabs") {
                    await cabalsModel.load()
                    await activityModel.load()
                }
                async let loadedCabals: Void = cabalsModel.load()
                async let loadedActivity: Void = activityModel.load()
                _ = await (loadedCabals, loadedActivity)
            }
        }
    }
}
