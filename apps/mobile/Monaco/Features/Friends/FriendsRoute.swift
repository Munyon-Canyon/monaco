import MonacoCore
import SwiftUI

nonisolated struct FriendsRoute: AppRoute {
    @MainActor func destination() -> FriendsScreen {
        FriendsScreen()
    }
}

struct FriendsScreen: View {
    var onSkip: (() -> Void)?
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.scenePhase) private var scenePhase
    @State private var model: FriendsOnMonacoModel?
    @State private var search: PeopleSearchModel?

    var body: some View {
        Group {
            if let model, let search {
                content(model: model, search: search)
            } else {
                Color.clear
            }
        }
        .monacoCanvas()
        .navigationTitle("Friends on Monaco")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { done }
        .task {
            guard model == nil else { return }
            let model = FriendsOnMonacoModel(
                api: environment.api,
                contacts: DeviceContactsSource(),
                defaultRegion: Locale.current.region?.identifier ?? "US"
            )
            self.model = model
            search = PeopleSearchModel(api: environment.api, clock: ContinuousClock())
            await model.loadIfGranted()
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
                PeopleSearchField(model: search)
                if search.isSearching {
                    PeopleSearchResults(model: search)
                } else if model.access == .granted {
                    FriendsOnMonacoView(model: model)
                } else {
                    ContactsExplainerView()
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .safeAreaInset(edge: .bottom) {
            if model.access != .granted {
                ContactsActions(model: model, onSkip: onSkip)
            }
        }
    }

    @ToolbarContentBuilder
    private var done: some ToolbarContent {
        if let onSkip, model?.access == .granted {
            ToolbarItem(placement: .topBarTrailing) {
                Button("Done", action: onSkip)
                    .accessibilityIdentifier("friends-done")
            }
        }
    }
}

private struct PeopleSearchField: View {
    @Bindable var model: PeopleSearchModel

    var body: some View {
        MonacoSearchField(placeholder: "Search by name or @handle", text: $model.query)
            .textInputAutocapitalization(.never)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("friends-search-field")
    }
}

#if DEBUG
final class FriendsSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-friendsHarness") else { return nil }
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
