import MonacoCore
import SwiftUI

nonisolated struct FriendsRoute: AppRoute {
    @MainActor func destination() -> FriendsScreen {
        FriendsScreen()
    }
}

struct FriendsScreen: View {
    static let title = "Find friends"

    var onSkip: (() -> Void)?
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.scenePhase) private var scenePhase
    @State private var model: FriendsOnMonacoModel?
    @State private var search: PeopleSearchModel?
    @State private var tab: FriendsTab = .discover
    @State private var following: UserProfileModel?

    var body: some View {
        Group {
            if let model, let search {
                content(model: model, search: search)
            } else {
                Color.clear
            }
        }
        .monacoCanvas()
        .navigationTitle(onSkip == nil ? Self.title : "")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { skip }
        .task {
            if model == nil {
                model = FriendsOnMonacoModel(
                    api: environment.api,
                    contacts: DeviceContactsSource(),
                    defaultRegion: Locale.current.region?.identifier ?? "US",
                    userID: environment.viewer?.userID,
                    now: { Date() }
                )
                search = PeopleSearchModel(api: environment.api, clock: ContinuousClock())
                following = environment.viewer.map { UserProfileModel(userID: $0.userID, api: environment.api) }
            }
            await model?.loadIfGranted()
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active, let model else { return }
            Task { await model.refreshAndLoad() }
        }
        .onChange(of: search?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: false)
        }
        .onChange(of: model?.toastTick) { _, _ in
            guard let message = model?.toast else { return }
            toasts.current = MonacoToast(message: message, isSuccess: false)
        }
    }

    private func content(model: FriendsOnMonacoModel, search: PeopleSearchModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if onSkip != nil {
                    ContactsFirstRunHeader(
                        showsExplainer: model.access != .granted, isDenied: model.access == .denied)
                }
                FriendsPeopleFrame(query: Bindable(search).query, tab: $tab, showsTabs: onSkip == nil) {
                    if onSkip == nil && tab == .following {
                        followingContent(search: search)
                    } else {
                        discoverContent(model: model, search: search)
                    }
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .safeAreaInset(edge: .bottom) {
            if model.access != .granted {
                ContactsActions(model: model, onSkip: onSkip)
            } else if let onSkip {
                ContactsDoneBar(done: onSkip)
            }
        }
    }

    @ViewBuilder
    private func discoverContent(model: FriendsOnMonacoModel, search: PeopleSearchModel) -> some View {
        if search.isSearching {
            PeopleSearchResults(model: search)
        } else if model.access == .granted {
            FriendsOnMonacoView(model: model)
        } else if onSkip == nil {
            ContactsExplainerView(isDenied: model.access == .denied)
        }
    }

    @ViewBuilder
    private func followingContent(search: PeopleSearchModel) -> some View {
        if let following {
            FriendsFollowingList(model: following, filter: search.normalized) {
                search.query = ""
            }
        } else {
            FriendsFollowingEmpty()
                .accessibilityIdentifier("friends-following-empty")
        }
    }

    @ToolbarContentBuilder
    private var skip: some ToolbarContent {
        FirstRunToolbar(
            signOut: nil,
            skip: onSkip.flatMap { onSkip in
                model?.access == .granted
                    ? nil
                    : FirstRunAction(title: "Skip", identifier: "friends-not-now", isDisabled: false, perform: onSkip)
            })
    }
}

#if DEBUG
final class FriendsSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-friendsHarness") else { return nil }
        let states: [String: FriendsSampleState] = [
            "results": .results, "following": .following, "followingEmpty": .followingEmpty,
            "noResults": .noResults, "loading": .loading, "error": .error,
        ]
        if arguments.indices.contains(flag + 1), let state = states[arguments[flag + 1]] {
            return AnyView(FriendsSampleScreen(state: state))
        }
        return AnyView(FriendsHarness())
    }
}

private struct FriendsHarness: View {
    var body: some View {
        NavigationStack {
            FriendsRoute().destination()
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}
#endif
