import MonacoAPI
import Synchronization

nonisolated final class SessionTokens: AccessTokenProvider, Sendable {
    private let privyToken: @Sendable () async -> String?
    private let refresh: @Sendable (String) async throws -> String?
    #if DEBUG
    private let devSession = Mutex<DevSession?>(nil)
    #endif

    convenience init(auth: PrivyAuthService) {
        self.init(
            privyToken: { await auth.accessToken },
            refresh: { try await auth.refreshedAccessToken(replacing: $0) }
        )
    }

    init(
        privyToken: @escaping @Sendable () async -> String?,
        refresh: @escaping @Sendable (String) async throws -> String?
    ) {
        self.privyToken = privyToken
        self.refresh = refresh
    }

    #if DEBUG
    func use(_ session: DevSession?) {
        devSession.withLock { $0 = session }
    }
    #endif

    func accessToken() async throws -> String? {
        #if DEBUG
        if let token = devSession.withLock({ $0?.token }) { return token }
        #endif
        return await privyToken()
    }

    func refreshedToken(replacing stale: String) async throws -> String? {
        #if DEBUG
        if devSession.withLock({ $0 != nil }) { return nil }
        #endif
        return try await refresh(stale)
    }
}
