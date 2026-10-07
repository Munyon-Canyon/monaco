import Foundation
import MonacoAPI
import Observation

public struct ReferralReferrer: Equatable, Sendable {
    public let userID: String
    public let handle: String

    public init(userID: String, handle: String) {
        self.userID = userID
        self.handle = handle
    }
}

public enum ReferralLookup: Equatable, Sendable {
    case referrer(ReferralReferrer)
    case unknown
}

@Observable
@MainActor
public final class ReferralLookupModel {
    public private(set) var state: LoadState<ReferralLookup> = .idle

    private let api: APIClient
    private let code: ReferralCode

    public init(api: APIClient, code: ReferralCode) {
        self.api = api
        self.code = code
    }

    public func load() async {
        if case .loaded = state {} else { state = .loading }
        do {
            let code = code.value
            let lookup = try await api.read { client in
                try await client.getReferral(path: .init(code: code)).ok.body.json
            }
            state = .loaded(
                lookup.referrer.map { .referrer(ReferralReferrer(userID: $0.userId, handle: $0.handle)) } ?? .unknown)
        } catch {
            state = .failed(APIError(error))
        }
    }
}
