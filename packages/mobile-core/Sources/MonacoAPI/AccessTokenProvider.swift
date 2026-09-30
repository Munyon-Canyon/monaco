/// Where `APIClient` gets the member's bearer token. `nil` means signed out.
public protocol AccessTokenProvider: Sendable {
    func accessToken() async throws -> String?
    /// A fresh token after the backend refused `replacing` with a 401, or `nil` when the
    /// session cannot be renewed.
    func refreshedToken(replacing: String) async throws -> String?
}
