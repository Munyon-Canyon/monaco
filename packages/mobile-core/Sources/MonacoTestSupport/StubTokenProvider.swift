import MonacoAPI

/// An `AccessTokenProvider` that hands out `token` and answers each refresh with the next
/// entry of `refreshes`, then `nil`.
public actor StubTokenProvider: AccessTokenProvider {
    private var token: String?
    private var refreshes: [String?]
    public private(set) var refreshed: [String] = []

    public init(token: String?, refreshes: [String?] = []) {
        self.token = token
        self.refreshes = refreshes
    }

    public func accessToken() -> String? {
        token
    }

    public func refreshedToken(replacing stale: String) -> String? {
        refreshed.append(stale)
        token = refreshes.isEmpty ? nil : refreshes.removeFirst()
        return token
    }
}
