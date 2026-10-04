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

    public func handleAvailability(_ handle: String) async throws -> HandleStatus {
        let reply = try await api.read { client in
            try await client.getHandleAvailability(path: .init(handle: handle)).ok.body.json
        }
        guard !reply.available else { return .available(handle) }
        return .unavailable(handle, reply.reason.map(HandleReason.init) ?? .invalid)
    }

    public func setHandle(_ handle: String, submission: IdempotentSubmission) async throws -> SessionProfile {
        let request = Components.Schemas.SetHandle(handle: handle)
        let data = try await api.sessionSubmit(submission, payload: request, operation: "putMeHandle") {
            client, key in
            _ = try await client.putMeHandle(.init(headers: .init(idempotencyKey: key), body: .json(request))).ok
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

extension HandleReason {
    init(_ reason: Components.Schemas.HandleAvailabilityReason) {
        switch reason {
        case .taken: self = .taken
        case .reserved: self = .reserved
        case .invalid: self = .invalid
        case .tooSoon: self = .tooSoon
        }
    }
}

#if DEBUG
public enum HandlePreviewAnswer: Sendable {
    case available
    case checking
    case checkFailed
    case saving
}

extension SessionAPI {
    public static func handlePreview(_ answer: HandlePreviewAnswer) -> SessionAPI {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let transport = HandlePreviewTransport(answer)
        return SessionAPI(api: APIClient(serverURL: serverURL, tokens: HandlePreviewTokens(), transport: transport))
    }
}

private struct HandlePreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct HandlePreviewTransport: ClientTransport {
    let answer: HandlePreviewAnswer

    init(_ answer: HandlePreviewAnswer) {
        self.answer = answer
    }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        switch (answer, operationID) {
        case (.checking, _), (.saving, Operations.PutMeHandle.id):
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case (.checkFailed, _):
            throw URLError(.notConnectedToInternet)
        default:
            let handle = request.path?.split(separator: "/").dropLast().last.map(String.init) ?? ""
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            return (response, HTTPBody(#"{"handle":"\#(handle)","available":true}"#))
        }
    }
}
#endif
