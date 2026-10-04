import MonacoAPI
import MonacoCore
import SwiftUI

@MainActor
protocol CabalsActionSource {
    func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
        -> Components.Schemas.Cabal
}

@MainActor
struct LiveCabalsActionSource: CabalsActionSource {
    let auth: PrivyAuthService
    let api: APIClient

    func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
        -> Components.Schemas.Cabal
    {
        let request = input.request
        return try await api.submit(submission, payload: request, operation: "postCabal") { client, key in
            try await client.postCabal(headers: .init(idempotencyKey: key), body: .json(request)).created.body.json
        }
    }
}
