import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public enum CabalActions: Equatable, Sendable {
    case loading
    case hidden
    case member(canPropose: Bool)
    case failed(APIError)
}

@Observable
@MainActor
public final class CabalActionsModel {
    public let cabalID: String
    public private(set) var actions: CabalActions = .loading
    public private(set) var cabalName: String?
    public private(set) var toast: String?

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public func load() async {
        generation += 1
        let mine = generation
        if !isShowingRow {
            actions = .loading
        }
        do {
            let cabalID = cabalID
            let cabal = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            guard mine == generation else { return }
            cabalName = cabal.name
            actions = cabal.me.map { .member(canPropose: $0.canVote) } ?? .hidden
        } catch {
            guard mine == generation else { return }
            let failure = APIError(error)
            if isShowingRow {
                toast = ToastCopy.message(for: failure)
            } else {
                actions = .failed(failure)
            }
        }
    }

    public func observe() async {
        let cabalHints = hints.hints(matching: .cabal(id: cabalID, what: nil))
        let accessHints = hints.hints(matching: .user(what: "cabal_access"))
        async let cabal: Void = reload(on: cabalHints) { hint in
            guard case .changed(_, let what, _) = hint else { return true }
            return what == "updated" || what == "members"
        }
        async let access: Void = reload(on: accessHints) { hint in
            if case .changed = hint { return true }
            return false
        }
        _ = await (cabal, access)
    }

    public func dismissToast() {
        toast = nil
    }

    private func reload(on stream: AsyncStream<Hint>, when wanted: @Sendable (Hint) -> Bool) async {
        for await hint in stream where wanted(hint) {
            if Task.isCancelled { return }
            await load()
        }
    }

    private var isShowingRow: Bool {
        switch actions {
        case .member, .hidden: true
        case .loading, .failed: false
        }
    }
}

#if DEBUG
extension CabalActionsModel {
    public static func preview(_ cabal: Components.Schemas.Cabal) -> CabalActionsModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return CabalActionsModel(
            cabalID: cabal.id,
            api: APIClient(
                serverURL: serverURL, tokens: CabalActionsPreviewTokens(),
                transport: CabalActionsPreviewTransport(cabal: cabal)),
            hints: CabalActionsPreviewHints()
        )
    }
}

private struct CabalActionsPreviewTransport: ClientTransport {
    let cabal: Components.Schemas.Cabal

    func send(
        _: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try JSONEncoder().encode(cabal)))
    }
}

private struct CabalActionsPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalActionsPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
