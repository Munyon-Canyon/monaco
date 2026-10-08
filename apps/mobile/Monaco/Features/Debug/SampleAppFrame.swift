#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct SampleAppFrame: View {
    @ObservedObject var auth: PrivyAuthService
    let sheet: (() -> AnyView)?
    @State private var environment: AppEnvironment
    @State private var showsSheet: Bool

    init(
        auth: PrivyAuthService,
        tab: MainTab = .home,
        routes: [any AppRoute] = [],
        store: AppSessionStore = AppSessionStore(),
        session: @MainActor (AppSessionStore) -> Void = SampleAppFrame.signedIn,
        sheet: (() -> AnyView)? = nil
    ) {
        self.auth = auth
        self.sheet = sheet
        session(store)
        let environment = AppEnvironment(
            auth: auth,
            tokens: SessionTokens(privyToken: { "sample-token" }, refresh: { _ in nil }),
            hints: SampleSilentHints(),
            sessionStore: store,
            isAuthenticated: { true },
            endAuthSession: {}
        )
        environment.navigator.selectedTab = tab
        for route in routes { environment.navigator.open(route, in: tab) }
        _environment = State(initialValue: environment)
        _showsSheet = State(initialValue: sheet != nil)
    }

    var body: some View {
        MainTabView()
            .environment(environment)
            .environment(environment.sessionStore)
            .sheet(isPresented: $showsSheet) { sheet?() }
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

nonisolated struct SampleSilentHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
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
    }

    let screen: Screen
    let arguments: [String]

    @MainActor @ViewBuilder func destination() -> some View {
        switch screen {
        case .chat: ChatSampleQA.screen(arguments: arguments)
        case .chatThread: ChatThreadSampleQA.screen()
        }
    }
}
#endif
