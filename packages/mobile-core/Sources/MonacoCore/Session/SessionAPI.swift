import Foundation
import MonacoAPI

public struct SessionAPI: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func openSession() async throws -> SessionProfile {
        let data = try await api.sessionBody { client in
            _ = try await client.postAuthSession().ok
        }
        return try SessionProfile(json: data)
    }

    public func me() async throws -> SessionProfile {
        let data = try await api.sessionBody { client in
            _ = try await client.getMe().ok
        }
        return try SessionProfile(json: data)
    }

    public func updateDisplayName(
        _ displayName: String,
        submission: IdempotentSubmission
    ) async throws -> SessionProfile {
        let request = Components.Schemas.UpdateProfileRequest(displayName: displayName)
        let data = try await api.sessionSubmit(submission, payload: request, operation: "patchMe") { client, key in
            _ = try await client.patchMe(.init(headers: .init(idempotencyKey: key), body: .json(request))).ok
        }
        return try SessionProfile(json: data)
    }

    public func uploadProfilePhoto(_ photo: Data, submission: IdempotentSubmission) async throws -> SessionProfile {
        let data = try await api.sessionSubmit(submission, payload: photo, operation: "postProfilePhoto") {
            client, key in
            let part = MultipartPart(
                payload: Operations.PostProfilePhoto.Input.Body.MultipartFormPayload.PhotoPayload(
                    body: HTTPBody(photo)
                ),
                filename: "photo"
            )
            _ = try await client.postProfilePhoto(
                .init(headers: .init(idempotencyKey: key), body: .multipartForm([.photo(part)]))
            ).ok
        }
        return try SessionProfile(json: data)
    }
}

public enum ProfileSaveFailure: Equatable, Sendable {
    case invalidName(String)
    case toast(String)

    public init(_ error: APIError) {
        if case .problem(let problem) = error, problem.code == .known(.displayNameInvalid) {
            self = .invalidName(problem.message)
        } else {
            self = .toast(ToastCopy.message(for: error))
        }
    }

    public var message: String {
        switch self {
        case .invalidName(let message), .toast(let message): message
        }
    }
}
