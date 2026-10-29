import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileTabsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileTabsLive()
    }
}

enum ProfileTabsSelection: Hashable, CaseIterable {
    case cabals
    case activity
}

struct ProfileTabsContent: View {
    let cabals: LoadState<[ProfileCabalRow]>
    let activity: LoadState<[MemberActivityRow]>
    let hasMoreActivity: Bool
    @Binding var selection: ProfileTabsSelection
    var retryCabals: () -> Void = {}
    var retryActivity: () -> Void = {}
    var findCabal: () -> Void = {}
    var loadMore: () -> Void = {}

    private var count: Int? {
        if case .loaded(let rows) = cabals { return ProfileCabalRows.memberCount(rows) }
        return nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoUnderlineTabs(
                ProfileTabsSelection.allCases, selection: $selection,
                count: { $0 == .cabals ? count : nil },
                identifier: { $0 == .cabals ? "profile-tab-cabals" : "profile-tab-activity" },
                label: { $0 == .cabals ? "Cabals" : "Activity" }
            )
            switch selection {
            case .cabals: cabalsBody
            case .activity: activityBody
            }
        }
    }

    @ViewBuilder private var cabalsBody: some View {
        switch cabals {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3).accessibilityIdentifier("profile-cabals-loading")
        case .failed:
            failure(identifier: "profile-cabals-error", retry: retryCabals)
        case .loaded(let rows) where rows.isEmpty:
            EmptyState(
                title: "No cabals yet", actionTitle: "Find a cabal", actionIdentifier: "profile-cabals-find",
                action: findCabal
            )
            .padding(.vertical, MonacoTheme.Space.l)
            .frame(maxWidth: .infinity)
        case .loaded(let rows):
            MonacoGroupedList {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    let last = index == rows.count - 1
                    switch row {
                    case .member(let member): ProfileCabalRowView(row: member, isLast: last)
                    case .invited(let invite): ProfileCabalRowView(invite: invite, isLast: last)
                    }
                }
            }
        }
    }

    @ViewBuilder private var activityBody: some View {
        switch activity {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .none).accessibilityIdentifier("profile-activity-loading")
        case .failed:
            failure(identifier: "profile-activity-error", retry: retryActivity)
        case .loaded(let rows) where rows.isEmpty:
            EmptyState(title: "Nothing yet. Your votes, proposals and replies show here.")
                .padding(.vertical, MonacoTheme.Space.l)
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("profile-activity-empty")
        case .loaded(let rows):
            MonacoGroupedList {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    ProfileActivityRowView(row: row, index: index, isLast: index == rows.count - 1)
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

private struct ProfileTabsLive: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var cabals: ProfileCabalsModel?
    @State private var activity: MemberActivityModel?
    @State private var selection = ProfileTabsSelection.cabals

    var body: some View {
        ProfileTabsContent(
            cabals: cabals?.state ?? .loading, activity: activity?.state ?? .loading,
            hasMoreActivity: activity?.hasMore ?? false, selection: $selection,
            retryCabals: { Task { await cabals?.load() } },
            retryActivity: { Task { await activity?.load() } },
            findCabal: { environment.navigator.selectedTab = .cabals },
            loadMore: { Task { await activity?.loadMore() } }
        )
        .task(id: environment.viewer?.userID) {
            guard let userID = environment.viewer?.userID else { return }
            let cabalsModel = ProfileCabalsModel(api: environment.api, hints: environment.hints)
            let activityModel = MemberActivityModel(userID: userID, api: environment.api)
            cabals = cabalsModel
            activity = activityModel
            await withTaskGroup(of: Void.self) { group in
                group.addTask { await cabalsModel.load() }
                group.addTask { await cabalsModel.observe() }
                group.addTask { await activityModel.load() }
            }
        }
        .onScreenVisibilityChange { cabals?.setVisible($0) }
    }
}
