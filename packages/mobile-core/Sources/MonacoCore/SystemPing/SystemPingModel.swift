import MonacoAPI
import Observation

@available(macOS 14, iOS 17, *)
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
