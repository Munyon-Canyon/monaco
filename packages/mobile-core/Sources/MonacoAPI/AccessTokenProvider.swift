public protocol AccessTokenProvider: Sendable {
    func accessToken() async throws -> String?
    func refreshedToken(replacing: String) async throws -> String?
    func endSession() async
}

extension AccessTokenProvider {
    public func endSession() async {}
}
