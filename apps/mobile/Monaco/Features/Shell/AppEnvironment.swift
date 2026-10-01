import MonacoAPI
import MonacoCore
import SwiftUI
import os

@Observable
@MainActor
final class AppEnvironment {
    let tokens: SessionTokens
    let api: APIClient
    let hints: any HintConnecting
    let auth: PrivyAuthService
    let navigator = AppNavigator()
    var viewer: Viewer?
    #if DEBUG
    private(set) var devSessionActive = false
    #endif

    private let privyAuthenticated: @MainActor () -> Bool
    private let endAuthSession: @MainActor () async -> Void
    private var isSigningOut = false

    var isSignedIn: Bool {
        #if DEBUG
        if devSessionActive { return true }
        #endif
        return privyAuthenticated()
    }

    init(
        auth: PrivyAuthService,
        tokens: SessionTokens? = nil,
        hints: any HintConnecting,
        isAuthenticated: (@MainActor () -> Bool)? = nil,
        endAuthSession: (@MainActor () async -> Void)? = nil
    ) {
        let tokens = tokens ?? SessionTokens(auth: auth)
        self.auth = auth
        self.tokens = tokens
        self.hints = hints
        self.api = APIClient(serverURL: Config.api.baseURL, tokens: tokens)
        self.privyAuthenticated = isAuthenticated ?? Self.privyIsAuthenticated(auth)
        self.endAuthSession = endAuthSession ?? { await auth.logout() }
        tokens.onSignedOut { [weak self] in
            Task { @MainActor in
                await self?.signOut()
            }
        }
    }

    convenience init() {
        let auth = PrivyAuthService()
        let tokens = SessionTokens(auth: auth)
        let stream = HintStream(
            serverURL: Config.api.baseURL,
            token: { try await tokens.accessToken() },
            refresh: { try await tokens.refreshedToken(replacing: $0) }
        )
        self.init(auth: auth, tokens: tokens, hints: LiveHintConnection(stream))
    }

    func sceneDidChange(_ phase: ScenePhase) async {
        switch phase {
        case .active:
            guard isSignedIn else { return }
            await hints.start()
            AppLogger.session.info("hint stream started")
        case .background:
            await hints.stop()
            AppLogger.session.info("hint stream stopped")
        default:
            break
        }
    }

    #if DEBUG
    func signIn(dev session: DevSession) async {
        tokens.use(session)
        devSessionActive = true
        viewer = Viewer(userID: session.userID, handle: nil)
        await hints.start()
        AppLogger.session.info("hint stream started")
    }
    #endif

    func signOut() async {
        guard !isSigningOut else { return }
        isSigningOut = true
        defer { isSigningOut = false }
        await hints.stop()
        AppLogger.session.info("hint stream stopped")
        #if DEBUG
        tokens.use(nil)
        devSessionActive = false
        #endif
        await endAuthSession()
        viewer = nil
    }

    private static func privyIsAuthenticated(_ auth: PrivyAuthService) -> @MainActor () -> Bool {
        {
            if case .authenticated = auth.phase, auth.accessToken != nil {
                return true
            }
            return false
        }
    }
}
