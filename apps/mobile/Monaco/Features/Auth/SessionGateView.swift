import MonacoCore
import SwiftUI

/// What the gates say when they can't let the member through. Shared with the sample harness, so
/// a screenshot shows what the app says.
enum SessionGateCopy {
    /// The saved sign-in couldn't be checked (offline). The member is still signed in.
    static let restoreFailedTitle = "Can't sign you in yet"
    /// Signed in, but the backend session didn't open. Not "Couldn't open Monaco": that is the
    /// generic message set under the title, and the screen used to say it twice.
    static let openFailedTitle = "Your account didn't load"
}

struct SessionGateView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(AppSessionStore.self) private var session

    var body: some View {
        Group {
            if let profile = session.profile {
                routed(profile)
            } else if let message = session.errorMessage, !session.isLoading {
                SessionFailureView(
                    title: SessionGateCopy.openFailedTitle, message: message, detail: nil,
                    onRetry: {
                        await session.bootstrap(auth: environment.auth, devSession: environment.skipsSessionOpen)
                    },
                    onSignOut: { await environment.signOut() }
                )
                .accessibilityIdentifier("sessionOpenFailedView")
            } else {
                SessionGateSkeleton()
            }
        }
        .task(id: environment.isSignedIn) {
            guard environment.isSignedIn else { return }
            await session.bootstrap(auth: environment.auth, devSession: environment.skipsSessionOpen)
        }
    }
    @ViewBuilder
    private func routed(_ profile: SessionProfile) -> some View {
        switch FirstRunGate.destination(for: profile) {
        case .nameSetup:
            OnboardingNameView(
                auth: environment.auth,
                save: { await session.updateDisplayName($0, auth: environment.auth, optimistic: false) },
                signOut: { await environment.signOut() }
            )
        case .app:
            MainTabView()
        case .session: SessionGateSkeleton()
        }
    }

}

#Preview {
    let environment = AppEnvironment()
    SessionGateView()
        .environment(environment)
        .environment(environment.sessionStore)
}

/// Why the app can't get past sign-in, and the two ways on: try again, or sign out. The restore
/// that couldn't reach Privy and the session the backend wouldn't open both end here.
///
/// Try again is the one filled button. Sign out is text under it: the way out has to be on
/// screen, but it is not what most members stuck here want.
struct SessionFailureView: View {
    let title: String
    let message: String
    /// Debug builds: what failed, in mono under the message.
    var detail: String? = nil
    let onRetry: () async -> Void
    let onSignOut: () async -> Void

    var body: some View {
        VStack(spacing: 0) {
            EmptyState(title: title, message: message)

            if let detail {
                Text(detail)
                    .font(MonacoTheme.Typo.dataCaption)
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, MonacoTheme.Space.l)
                    .padding(.bottom, MonacoTheme.Space.m)
                    .accessibilityIdentifier("sessionErrorDebugDetail")
            }

            Button {
                Task { await onRetry() }
            } label: {
                Text("Try again")
                    .frame(minWidth: 140)
            }
            .buttonStyle(.monacoPrimary)

            Button {
                Task { await onSignOut() }
            } label: {
                Text("Sign out")
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(minHeight: 44)
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.top, MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
    }
}

/// Home's shape while the session opens: the figure on the paper, the curve's slot, the balance
/// line and a ruled table of cabals, under the same inline bar Home has with a place for the
/// avatar. When the tabs arrive, the figure is already where Home puts it.
///
/// It used to be three rounded cards, the shape of a screen the app no longer has.
struct SessionGateSkeleton: View {
    var body: some View {
        NavigationStack {
            HomeShapedSkeleton()
                .navigationTitle("")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        SkeletonBlock(width: 32, height: 32, radius: 16)
                            .frame(width: 44, height: 44)
                    }
                }
        }
    }
}

/// The loading shape Home and the gate share: the figure, the past hour's slot, the balance
/// line, three cabal rows. One view, so the rows do not visibly change as the gate hands over
/// to Home.
struct HomeShapedSkeleton: View {
    /// Where a row's rule starts: under the text, past the 44pt mark (see `MonacoRow`).
    private static let rowRuleInset = MonacoTheme.Space.m + 44 + MonacoTheme.Space.sm

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
            figure
            balance
            cabals
        }
        .padding(.top, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading")
        .monacoCanvas()
    }

    /// "Your money in cabals", the figure and its badge, each block centred in the line its
    /// text will take; then the past hour's slot, edge to edge, with its stamp.
    private var figure: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                SkeletonBlock(width: 140, height: 14)
                    .frame(height: 18)
                SkeletonBlock(width: 200, height: 44)
                    .frame(height: 60)
                SkeletonBlock(width: 120, height: 26, radius: 13)
            }
            .padding(.horizontal, MonacoTheme.Space.m)

            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                SkeletonBlock(height: 92, radius: 0)
                SkeletonBlock(width: 72, height: 12)
                    .frame(height: 16)
                    .padding(.horizontal, MonacoTheme.Space.m)
            }
        }
    }

    /// The account balance: one ruled line with its coin, and the two text actions under it.
    private var balance: some View {
        MonacoGroupedList {
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 40, height: 40, radius: 20)
                    .frame(width: 44, height: 44)
                SkeletonBlock(width: 132, height: 14)
                Spacer(minLength: MonacoTheme.Space.sm)
                SkeletonBlock(width: 76, height: 14)
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 60)

            HStack(spacing: MonacoTheme.Space.l) {
                SkeletonBlock(width: 88, height: 12)
                SkeletonBlock(width: 68, height: 12)
            }
            .frame(minHeight: 44)
            .padding(.leading, Self.rowRuleInset)
            .padding(.bottom, MonacoTheme.Space.xs)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    /// "Your cabals": a header over three ruled rows, each a mark, a name over its pot, and the
    /// member's slice over its change.
    private var cabals: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            SkeletonBlock(width: 128, height: 18)
                .frame(height: 27)
                .padding(.horizontal, MonacoTheme.Space.m)

            MonacoGroupedList {
                ForEach(0..<3, id: \.self) { index in
                    HStack(spacing: MonacoTheme.Space.sm) {
                        SkeletonBlock(width: 44, height: 44, radius: MonacoTheme.Radius.tile)
                        VStack(alignment: .leading, spacing: 6) {
                            SkeletonBlock(width: 144, height: 14)
                            SkeletonBlock(width: 88, height: 12)
                        }
                        Spacer(minLength: MonacoTheme.Space.sm)
                        VStack(alignment: .trailing, spacing: 6) {
                            SkeletonBlock(width: 68, height: 14)
                            SkeletonBlock(width: 40, height: 12)
                        }
                    }
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .padding(.vertical, MonacoTheme.Space.s)
                    .frame(minHeight: 60)
                    .overlay(alignment: .bottom) {
                        if index < 2 {
                            MonacoRule()
                                .padding(.leading, Self.rowRuleInset)
                        }
                    }
                }
            }
        }
    }
}
