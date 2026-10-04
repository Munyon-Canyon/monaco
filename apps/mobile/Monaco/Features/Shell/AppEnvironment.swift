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
    let linking: any AccountLinking
    let navigator = AppNavigator()
    let sessionStore: AppSessionStore
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
        sessionStore: AppSessionStore? = nil,
        isAuthenticated: (@MainActor () -> Bool)? = nil,
        endAuthSession: (@MainActor () async -> Void)? = nil
    ) {
        let tokens = tokens ?? SessionTokens(auth: auth)
        self.auth = auth
        self.tokens = tokens
        self.hints = hints
        let api = APIClient(serverURL: Config.api.baseURL, tokens: tokens)
        self.api = api
        self.linking = PrivyAccountLinker(privy: auth.privy, api: api)
        self.sessionStore =
            sessionStore
            ?? AppSessionStore(
                apiClient: MonacoAPIClient(), sessions: SessionAPI(api: api)
            )
        self.privyAuthenticated = isAuthenticated ?? Self.privyIsAuthenticated(auth)
        self.endAuthSession = endAuthSession ?? { await auth.logout() }
        self.sessionStore.onProfileChange = { [weak self] next in
            self?.viewer = next.map(Viewer.init)
        }
        tokens.onSignedOut { [weak self] in
            Task { @MainActor in
                await self?.signOut()
            }
        }
        auth.onSessionEnded = { [weak self] in
            self?.clearSignedInState()
        }
    }

    convenience init() {
        let auth = PrivyAuthService()
        let tokens = SessionTokens(auth: auth)
        let stream = HintStream(
            serverURL: Config.api.baseURL,
            token: { try await tokens.accessToken() },
            refresh: { try await tokens.refreshedToken(replacing: $0) },
            endSession: { rejectedToken in
                guard let rejectedToken else { return }
                await tokens.endSession(rejectedToken: rejectedToken)
            }
        )
        self.init(auth: auth, tokens: tokens, hints: LiveHintConnection(stream))
    }

    func sceneDidChange(_ phase: ScenePhase) async {
        switch phase {
        case .active:
            guard isSignedIn else { return }
            await hints.start()
            await sessionStore.noteForeground(auth: auth)
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
        auth.adoptDevAccessToken(session.token)
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
        clearSignedInState()
        await hints.stop()
        AppLogger.session.info("hint stream stopped")
        #if DEBUG
        tokens.use(nil)
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

    private func clearSignedInState() {
        navigator.reset()
        sessionStore.reset()
        #if DEBUG
        tokens.use(nil)
        devSessionActive = false
        #endif
    }

    var skipsSessionOpen: Bool {
        #if DEBUG
        devSessionActive
        #else
        false
        #endif
    }
}
