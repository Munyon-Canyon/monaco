import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class SystemPingModel {
    public private(set) var state: LoadState<Components.Schemas.Ping> = .idle
    public private(set) var isSending = false

    private let api: APIClient
    private let hints: any HintSource
    private let submission = IdempotentSubmission()
    private var pingID: String?
    private var generation = 0

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public func load() async {
        generation += 1
        let mine = generation
        guard let pingID else { return }
        if !isShowingPing {
            state = .loading
        }
        do {
            let ping = try await api.read { client in
                try await client.getSystemPing(path: .init(id: pingID)).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(ping)
        } catch {
            guard mine == generation else { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        for await _ in hints.hints(matching: .user(what: "ping_echoed")) {
            if Task.isCancelled { return }
            await load()
        }
    }

    private var isShowingPing: Bool {
        if case .loaded = state { return true }
        return false
    }

    public func send(note: String) async {
        if isSending { return }
        isSending = true
        defer { isSending = false }
        let body = Components.Schemas.PingRequest(note: note)
        do {
            let created = try await api.submit(submission, payload: body, operation: "postSystemPing") { client, key in
                try await client.postSystemPing(
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).created.body.json
            }
            pingID = created.id
            await load()
        } catch {
            state = .failed(APIError(error))
        }
    }
}

#if DEBUG
extension SystemPingModel {
    public static func preview() -> SystemPingModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return SystemPingModel(
            api: APIClient(
                serverURL: serverURL,
                tokens: SystemPingDisconnectedTokens(),
                transport: SystemPingDisconnectedTransport()
            ),
            hints: SystemPingDisconnectedHints()
        )
    }
}

private struct SystemPingDisconnectedTransport: ClientTransport {
    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let status: HTTPResponse.Status = request.method == .post ? .created : .ok
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (
            response,
            HTTPBody(#"{"id":"00000000-0000-4000-8000-000000000001","note":"hi","echoed":true}"#)
        )
    }
}

private struct SystemPingDisconnectedTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { nil }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct SystemPingDisconnectedHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            continuation.finish()
        }
    }
}
#endif
