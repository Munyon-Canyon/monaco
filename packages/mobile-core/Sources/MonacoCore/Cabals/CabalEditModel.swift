import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalEditModel {
    public enum SaveOutcome: Equatable, Sendable {
        case saved
        case unchanged
        case failed(String)
    }

    public let cabalID: String
    public private(set) var state: LoadState<Components.Schemas.Cabal> = .idle
    public private(set) var isSaving = false
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0

    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private let submission = IdempotentSubmission()
    private let voterSubmission = IdempotentSubmission()
    private var generation = 0

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
        let hook = ReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public var cabal: Components.Schemas.Cabal? {
        if case .loaded(let cabal) = state { return cabal }
        return nil
    }

    public var isCreator: Bool {
        cabal?.me?.role == "creator"
    }

    public var settings: CabalSettings? {
        cabal.map(CabalSettings.init)
    }

    public var voterChoice: CabalVoterChoice? {
        cabal.map(CabalVoterChoice.init)
    }

    public func patch(for edited: CabalSettings) -> Components.Schemas.UpdateCabalRequest? {
        guard let cabal else { return nil }
        return CabalRulesDiff.patch(from: CabalSettings(cabal), to: edited)
    }

    public func load() async {
        generation += 1
        let mine = generation
        if cabal == nil {
            state = .loading
        }
        do {
            let cabalID = cabalID
            let loaded = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(loaded)
            lastError = nil
        } catch {
            guard mine == generation else { return }
            if cabal == nil {
                state = .failed(APIError(error))
            } else {
                lastError = APIError(error)
                failureTick += 1
            }
        }
    }

    public func observe() async {
        await withTaskGroup(of: Void.self) { group in
            for what in ["updated", "members"] {
                let stream = hints.hints(matching: .cabal(id: cabalID, what: what))
                group.addTask { await self.refresher.observe(stream) }
            }
        }
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func save(_ edited: CabalSettings) async -> SaveOutcome {
        guard !isSaving, let body = patch(for: edited) else { return .unchanged }
        guard !body.isEmpty else { return .unchanged }
        isSaving = true
        defer { isSaving = false }
        let cabalID = cabalID
        do {
            let saved = try await api.submit(submission, payload: body, operation: "patchCabal") { client, key in
                try await client.patchCabal(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).ok.body.json
            }
            generation += 1
            state = .loaded(saved)
            return .saved
        } catch {
            return .failed(ToastCopy.message(for: APIError(error)))
        }
    }

    public func saveVoters(_ choice: CabalVoterChoice) async -> SaveOutcome {
        guard !isSaving, let cabal else { return .unchanged }
        let creatorID = cabal.creator.userId
        let body = choice.patch(creatorID: creatorID)
        guard body != CabalVoterChoice(cabal).patch(creatorID: creatorID) else { return .unchanged }
        isSaving = true
        defer { isSaving = false }
        let cabalID = cabalID
        do {
            let saved = try await api.submit(voterSubmission, payload: body, operation: "patchCabal") { client, key in
                try await client.patchCabal(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).ok.body.json
            }
            generation += 1
            state = .loaded(saved)
            return .saved
        } catch {
            let error = APIError(error)
            if case .problem(let problem) = error, problem.code == .known(.invalidInput) {
                await load()
                return .failed("Someone you picked isn't in the cabal anymore.")
            }
            return .failed(ToastCopy.message(for: error))
        }
    }
}

@MainActor
private final class ReloadHook {
    var run: (@MainActor () async -> Void)?
}

#if DEBUG
extension CabalEditModel {
    public static func preview(_ cabal: Components.Schemas.Cabal) -> CabalEditModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return CabalEditModel(
            cabalID: cabal.id,
            api: APIClient(
                serverURL: serverURL, tokens: CabalPreviewTokens(), transport: CabalPreviewTransport(cabal: cabal)),
            hints: CabalPreviewHints()
        )
    }
}

private actor CabalPreviewTransport: ClientTransport {
    private var cabal: Components.Schemas.Cabal

    init(cabal: Components.Schemas.Cabal) {
        self.cabal = cabal
    }

    func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if request.method == .patch, let body {
            let data = try await Data(collecting: body, upTo: 64 * 1024)
            apply(try JSONDecoder().decode(Components.Schemas.UpdateCabalRequest.self, from: data))
        }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return (response, HTTPBody(try encoder.encode(cabal)))
    }

    private func apply(_ change: Components.Schemas.UpdateCabalRequest) {
        if let name = change.name { cabal.name = name }
        if let joinMode = change.joinMode { cabal.rules.joinMode = joinMode }
        if let voterMode = change.voterMode {
            cabal.rules.voterMode = voterMode
            let voters = Set(change.voterIds ?? [])
            for index in cabal.members.indices {
                cabal.members[index].canVote = voterMode != "list" || voters.contains(cabal.members[index].userId)
            }
        }
        if let threshold = change.threshold { cabal.rules.threshold = threshold }
        if let expiry = change.proposalExpirySeconds { cabal.rules.proposalExpirySeconds = expiry }
    }
}

private struct CabalPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
