#if DEBUG
import Foundation
import MonacoAPI

extension AccountActivityModel {
    public enum PreviewScenario: Sendable {
        case full
        case ready
        case pending
        case empty
        case loading
        case error
    }

    public static func preview(empty: Bool, clock: @escaping @Sendable () -> Date) -> AccountActivityModel {
        preview(empty ? .empty : .full, clock: clock)
    }

    public static func preview(
        _ scenario: PreviewScenario, clock: @escaping @Sendable () -> Date
    ) -> AccountActivityModel {
        AccountActivityModel(
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: AccountActivityPreviewTokens(),
                transport: AccountActivityPreviewTransport(behavior: behavior(scenario))),
            hints: AccountActivityPreviewHints(),
            clock: clock)
    }

    private static func behavior(_ scenario: PreviewScenario) -> AccountActivityPreviewTransport.Behavior {
        switch scenario {
        case .full:
            .pages([nil: .sampleFirst, Components.Schemas.UserTxnPage.sampleCursor: .sampleSecond])
        case .ready:
            .pages([nil: Components.Schemas.UserTxnPage(items: referenceRows(withdrawal: .settled), nextCursor: nil)])
        case .pending:
            .pages([nil: Components.Schemas.UserTxnPage(items: referenceRows(withdrawal: .pending), nextCursor: nil)])
        case .empty:
            .pages([nil: .sampleEmpty])
        case .loading:
            .hang
        case .error:
            .fail
        }
    }

    private static func referenceRows(
        withdrawal: Components.Schemas.UserTxn.StatusPayload
    ) -> [Components.Schemas.UserTxn] {
        let bigBallers = Components.Schemas.UserTxn.CabalPayload(id: "sample-big-ballers", name: "Big Ballers")
        let day: TimeInterval = 86_400
        let base: TimeInterval = 1_790_996_400
        return [
            Components.Schemas.UserTxn(
                id: "reference-deposit", kind: .deposit, status: .settled, usdcMicros: "100000000", cabal: nil,
                txSignature: "reference-deposit-signature", createdAt: Date(timeIntervalSince1970: base)),
            Components.Schemas.UserTxn(
                id: "reference-fund", kind: .fund, status: .settled, usdcMicros: "-250000000", cabal: bigBallers,
                txSignature: "reference-fund-signature", createdAt: Date(timeIntervalSince1970: base - day)),
            Components.Schemas.UserTxn(
                id: "reference-withdrawal", kind: .withdrawal, status: withdrawal, usdcMicros: "-100000000",
                cabal: nil, txSignature: withdrawal == .pending ? nil : "reference-withdrawal-signature",
                createdAt: Date(timeIntervalSince1970: base - 2 * day)),
        ]
    }
}

private struct AccountActivityPreviewTransport: ClientTransport {
    enum Behavior {
        case pages([String?: Components.Schemas.UserTxnPage])
        case hang
        case fail
    }

    struct Offline: Error {}

    let behavior: Behavior

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let pages: [String?: Components.Schemas.UserTxnPage]
        switch behavior {
        case .pages(let known):
            pages = known
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw Offline()
        case .fail:
            throw Offline()
        }
        let cursor = URLComponents(string: request.path ?? "")?.queryItems?.first { $0.name == "cursor" }?.value
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(pages[cursor] ?? .sampleEmpty)))
    }
}

private struct AccountActivityPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct AccountActivityPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
