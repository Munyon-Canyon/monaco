#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct SampleAppFrame: View {
    @ObservedObject var auth: PrivyAuthService
    let sheet: (() -> AnyView)?
    private let tab: MainTab
    private let presentsDeposit: Bool
    private let routes: [any AppRoute]
    private let store: AppSessionStore
    private let session: @MainActor (AppSessionStore) -> Void
    private let hints: any HintConnecting
    @State private var environment: AppEnvironment?
    @State private var showsSheet: Bool

    init(
        auth: PrivyAuthService,
        tab: MainTab = .home,
        routes: [any AppRoute] = [],
        presentsDeposit: Bool = false,
        store: AppSessionStore = AppSessionStore(),
        session: @escaping @MainActor (AppSessionStore) -> Void = SampleAppFrame.signedIn,
        sheet: (() -> AnyView)? = nil,
        hints: any HintConnecting = SampleSilentHints()
    ) {
        self.hints = hints
        self.auth = auth
        self.tab = tab
        self.routes = routes
        self.presentsDeposit = presentsDeposit
        self.store = store
        self.session = session
        self.sheet = sheet
        _showsSheet = State(initialValue: sheet != nil)
    }

    var body: some View {
        Group {
            if let environment {
                MainTabView()
                    .modifier(SampleScrollModifier(anchor: SampleScrollAnchor.requested(by: SampleScrollAnchor.flag)))
                    .sheet(isPresented: $showsSheet) { sheet?() }
                    .environment(environment)
                    .environment(environment.sessionStore)
                    .environment(
                        \.scrubSelectionPersists, ProcessInfo.processInfo.arguments.contains("-MonacoScrubHolds")
                    )
            } else {
                MonacoTheme.canvas.ignoresSafeArea()
            }
        }
        .onAppear {
            guard environment == nil else { return }
            environment = makeEnvironment()
        }
    }

    private func makeEnvironment() -> AppEnvironment {
        session(store)
        let made = AppEnvironment(
            auth: auth,
            tokens: SessionTokens(privyToken: { "sample-token" }, refresh: { _ in nil }),
            hints: hints,
            sessionStore: store,
            gifs: SampleGifProvider(),
            defaults: Self.throwawayDefaults(),
            isAuthenticated: { true },
            endAuthSession: {}
        )
        if tab == .profile, let userID = store.profile?.userID { made.viewer = Viewer(userID: userID, handle: nil) }
        made.navigator.selectedTab = tab
        for route in routes { made.navigator.open(route, in: tab) }
        if presentsDeposit { made.navigator.presentDeposit() }
        return made
    }

    private static func throwawayDefaults() -> UserDefaults {
        let suite = "MonacoSampleHarness"
        UserDefaults(suiteName: suite)?.removePersistentDomain(forName: suite)
        return UserDefaults(suiteName: suite) ?? .standard
    }

    @MainActor static func signedIn(_ store: AppSessionStore) {
        store.isLoading = false
        store.profile = ProfileSampleHarness.sampleProfile(
            userID: Components.Schemas.Me.sample.id,
            displayName: "Logan Norman",
            photoURL: nil
        )
        store.hasLoaded = true
    }

    @MainActor static func loading(_ store: AppSessionStore) {
        store.isLoading = true
    }
}

private struct SampleScrollModifier: ViewModifier {
    let anchor: UnitPoint?

    func body(content: Content) -> some View {
        if let anchor {
            content.defaultScrollAnchor(anchor)
        } else {
            content
        }
    }
}

nonisolated struct SampleSilentHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
    func start() async {}
    func stop() async {}
}

nonisolated struct SampleResyncHints: HintConnecting {
    let delay: Duration

    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            let task = Task {
                try? await Task.sleep(for: delay)
                continuation.yield(.resync)
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    func start() async {}
    func stop() async {}
}

enum SampleImage {
    @MainActor
    static func flat(_ tint: MonacoTheme.CabalTint, name: String) -> URL? {
        let size = CGSize(width: 256, height: 256)
        let image = UIGraphicsImageRenderer(size: size).image { context in
            UIColor(tint.fill).setFill()
            context.fill(CGRect(origin: .zero, size: size))
            UIColor.white.withAlphaComponent(0.85).setFill()
            context.cgContext.fillEllipse(in: CGRect(x: 78, y: 62, width: 100, height: 100))
            UIColor.white.withAlphaComponent(0.55).setFill()
            context.fill(CGRect(x: 48, y: 176, width: 160, height: 22))
        }
        guard let data = image.pngData() else { return nil }
        let url = FileManager.default.temporaryDirectory.appending(path: "monaco-sample-\(name).png")
        do {
            try data.write(to: url, options: .atomic)
            return url
        } catch {
            return nil
        }
    }
}
nonisolated struct SampleScreenRoute: AppRoute {
    enum Screen: Hashable, Sendable {
        case chat
        case chatThread
        case proposeBuy
        case proposeReview(ProposeKind)
    }

    let screen: Screen
    let arguments: [String]

    @MainActor @ViewBuilder func destination() -> some View {
        switch screen {
        case .chat: ChatSampleQA.screen(arguments: arguments)
        case .chatThread: ChatThreadSampleQA.screen()
        case .proposeBuy: ProposeBuyStockView(cabalID: GroupDetailSampleData.cabalID)
        case .proposeReview(let kind): ProposeReviewSample(kind: kind)
        }
    }
}
#endif
