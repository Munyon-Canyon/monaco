public protocol AccessTokenProvider: Sendable {
    func accessToken() async throws -> String?
    func refreshedToken(replacing: String) async throws -> String?
    func endSession(rejectedToken: String) async
    func endSession() async
    func accountDeleted(rejectedToken: String) async
}

extension AccessTokenProvider {
    public func endSession(rejectedToken _: String) async { await endSession() }
    public func endSession() async {}
    public func accountDeleted(rejectedToken: String) async { await endSession(rejectedToken: rejectedToken) }
}
