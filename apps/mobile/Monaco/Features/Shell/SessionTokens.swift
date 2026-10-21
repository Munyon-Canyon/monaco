import MonacoAPI
import Synchronization

nonisolated final class SessionTokens: AccessTokenProvider, Sendable {
    private let privyToken: @Sendable () async -> String?
    private let refresh: @Sendable (String) async throws -> String?
    private let fresh: @Sendable () async throws -> String?
    private let signedOutHandler = Mutex<(@Sendable () -> Void)?>(nil)
    private let accountDeletedHandler = Mutex<(@Sendable () -> Void)?>(nil)
    #if DEBUG
    private let devSession = Mutex<DevSession?>(nil)
    #endif

    convenience init(auth: PrivyAuthService) {
        self.init(
            privyToken: { await auth.accessToken },
            refresh: { try await auth.refreshedAccessToken(replacing: $0) },
            fresh: { try await auth.freshSessionToken() }
        )
    }

    init(
        privyToken: @escaping @Sendable () async -> String?,
        refresh: @escaping @Sendable (String) async throws -> String?,
        fresh: (@Sendable () async throws -> String?)? = nil
    ) {
        self.privyToken = privyToken
        self.refresh = refresh
        self.fresh = fresh ?? { await privyToken() }
    }

    #if DEBUG
    func use(_ session: DevSession?) {
        devSession.withLock { $0 = session }
    }
    #endif

    func onSignedOut(_ handler: @escaping @Sendable () -> Void) {
        signedOutHandler.withLock { $0 = handler }
    }

    func onAccountDeleted(_ handler: @escaping @Sendable () -> Void) {
        accountDeletedHandler.withLock { $0 = handler }
    }

    func accessToken() async throws -> String? {
        #if DEBUG
        if let token = devSession.withLock({ $0?.token }) { return token }
        #endif
        return await privyToken()
    }

    func freshAccessToken() async throws -> String? {
        #if DEBUG
        if let token = devSession.withLock({ $0?.token }) { return token }
        #endif
        return try await fresh()
    }

    func endSession() async {
        notifySignedOut()
    }

    func endSession(rejectedToken: String) async {
        if await isCurrent(rejectedToken) { notifySignedOut() }
    }

    func accountDeleted(rejectedToken: String) async {
        if await isCurrent(rejectedToken) { accountDeletedHandler.withLock { $0 }?() }
    }

    func refreshedToken(replacing stale: String) async throws -> String? {
        #if DEBUG
        if let dev = devSession.withLock({ $0 }) {
            if dev.token == stale { notifySignedOut() }
            return nil
        }
        #endif
        let current = await privyToken()
        let fresh = try await refresh(stale)
        if fresh == nil, current == stale {
            notifySignedOut()
        }
        return fresh
    }

    private func isCurrent(_ token: String) async -> Bool {
        #if DEBUG
        if let dev = devSession.withLock({ $0 }) { return dev.token == token }
        #endif
        return await privyToken() == token
    }

    private func notifySignedOut() {
        signedOutHandler.withLock { $0 }?()
    }
}
