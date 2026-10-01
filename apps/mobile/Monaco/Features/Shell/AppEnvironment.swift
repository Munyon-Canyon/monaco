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

    private let privyAuthenticated: @MainActor () -> Bool
    private let endAuthSession: @MainActor () async -> Void

    var isSignedIn: Bool {
        privyAuthenticated()
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

    func signOut() async {
        await hints.stop()
        AppLogger.session.info("hint stream stopped")
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
