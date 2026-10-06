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
    @Environment(\.scenePhase) private var scenePhase
    @State private var model: FriendsOnMonacoModel?

    var body: some View {
        Group {
            if let model {
                if model.access == .granted {
                    FriendsOnMonacoView(model: model, onDone: onSkip)
                } else {
                    ContactsExplainerView(model: model, onSkip: onSkip)
                }
            } else {
                Color.clear
            }
        }
        .task {
            guard model == nil else { return }
            model = FriendsOnMonacoModel(
                api: environment.api,
                contacts: DeviceContactsSource(),
                defaultRegion: Locale.current.region?.identifier ?? "US"
            )
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active, let model else { return }
            Task { await model.refreshAndLoad() }
        }
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
