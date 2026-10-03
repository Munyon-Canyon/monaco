import MonacoAPI
import MonacoCore
import SwiftUI

@MainActor
protocol CabalsActionSource {
    func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
        -> Components.Schemas.Cabal

    func joinGroup(groupId: String) async throws -> JoinGroupOutcome
}

@MainActor
struct LiveCabalsActionSource: CabalsActionSource {
    let auth: PrivyAuthService
    let api: APIClient
    private let apiClient = MonacoAPIClient()

    init(auth: PrivyAuthService, api: APIClient) {
        self.auth = auth
        self.api = api
    }

    func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
        -> Components.Schemas.Cabal
    {
        let request = input.request
        return try await api.submit(submission, payload: request, operation: "postCabal") { client, key in
            try await client.postCabal(headers: .init(idempotencyKey: key), body: .json(request)).created.body.json
        }
    }

    func joinGroup(groupId: String) async throws -> JoinGroupOutcome {
        guard let token = auth.accessToken else { throw MonacoAPIError.missingAccessToken }
        return try await apiClient.joinGroup(accessToken: token, groupId: groupId)
    }
}
