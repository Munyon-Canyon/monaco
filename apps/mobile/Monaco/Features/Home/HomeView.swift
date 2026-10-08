import MonacoCore
import SwiftUI
import os

/// What Home is showing right now. One value instead of a ladder of optionals, so the
/// screen cannot fall through to a fabricated "$0.00" dashboard (#327) and every state has
/// exactly one branch.
enum HomeScreenState: Equatable {
    case loading
    case loaded
    case failed(String)

    /// Data wins over an error: once a refresh lands the screen is real, and a refresh that fails
    /// with it on screen is reported by the toast in `body`, not by replacing it (#278).
    static func resolve(hasLoaded: Bool, errorMessage: String?) -> HomeScreenState {
        if hasLoaded { return .loaded }
        if let errorMessage { return .failed(errorMessage) }
        return .loading
    }
}

/// Home dashboard shell.
struct HomeView: View {
    @ObservedObject var auth: PrivyAuthService
    @Binding var selectedTab: MainTab
    @Environment(AppSessionStore.self) private var session

    @State private var isRetrying = false
    @State private var toast: MonacoToast?

    var body: some View {
        Group {
            switch HomeScreenState.resolve(hasLoaded: session.hasLoaded, errorMessage: session.errorMessage) {
            case .loading:
                // Skeleton until the first refresh lands (#217: session and dashboard load separately).
                HomeSkeletonView()
            case .loaded:
                dashboardScroll
            case .failed:
                failedScroll()
            }
        }
        .monacoCanvas()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                profileButton
            }
        }
        .refreshable {
            await pullToRefresh()
        }
        .monacoToast($toast)
        .monacoFrameStats("Home")
    }

    /// The viewer's photo (or initials) in the corner; tapping it switches to the Profile tab.
    /// `MainTabView` plays the selection haptic for every tab change, this one included.
    private var profileButton: some View {
        Button {
            selectedTab = .profile
        } label: {
            MonacoAvatar(
                photoURL: session.profile?.photoURL?.absoluteString,
                displayName: session.profile?.displayName ?? "",
                size: 32,
                seed: session.profile?.userID
            )
            .frame(width: 44, height: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Profile")
        .accessibilityIdentifier("home-profile-avatar")
    }

    private var dashboardScroll: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {}
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.xl)
        }
    }

    /// The failed state lives in a ScrollView, so the "pull down to try again" the store asks
    /// for is a gesture this screen actually has (#278).
    private func failedScroll() -> some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.s) {
                MonacoErrorRow(thing: "Home", identifier: "home-error") { Task { await retryLoad() } }
                    .disabled(isRetrying)
                if isRetrying {
                    ProgressView()
                        .tint(MonacoTheme.ink)
                        .accessibilityLabel("Loading")
                }
            }
            .frame(maxWidth: .infinity)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
        }
        .scrollBounceBehavior(.always)
        .accessibilityIdentifier("home-error")
    }

    private func retryLoad() async {
        guard !isRetrying else { return }
        isRetrying = true
        await refreshHome()
        isRetrying = false
    }

    private func pullToRefresh() async {
        await refreshHome()
        // A refresh the member let go of early is cancelled, and the store returns on
        // `isRequestCancellation` without touching `errorMessage` — so a message left over
        // from an earlier read would raise this toast for a read that merely stopped.
        // See "Needs from other areas": a per-refresh outcome would settle it properly.
        guard !Task.isCancelled else { return }
        // `refresh` clears the message when it succeeds, so anything left is this read failing.
        // With the board already on screen nothing else would say so.
        if session.hasLoaded, session.errorMessage != nil {
            toast = MonacoToast(message: "Couldn't refresh just now")
        }
    }

    private func refreshHome() async {
        await session.refresh(auth: auth)
    }

}

/// The loading shape Home shares with the session gate, under Home's own navigation bar.
private struct HomeSkeletonView: View {
    var body: some View {
        ScrollView {
            HomeShapedSkeleton()
        }
        .accessibilityIdentifier("home-loading")
    }
}

#Preview {
    let session = AppSessionStore()
    session.hasLoaded = true
    return NavigationStack {
        HomeView(auth: PrivyAuthService(), selectedTab: .constant(.home))
            .environment(session)
            .monacoRootAppearance()
    }
}
