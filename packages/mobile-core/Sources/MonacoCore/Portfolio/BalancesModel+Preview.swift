#if DEBUG
import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

extension BalancesModel {
    public enum PreviewScenario: Sendable {
        case ready
        case cashOnly
        case loading
        case failed
    }

    public static func preview(_ scenario: PreviewScenario) -> BalancesModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let api = APIClient(
            serverURL: serverURL, tokens: BalancesPreviewTokens(),
            transport: BalancesPreviewTransport(scenario: scenario))
        return BalancesModel(
            portfolio: PortfolioModel(api: api, hints: BalancesPreviewHints()),
            balance: BalanceSource(api: api, hints: BalancesPreviewHints()))
    }
}

extension Components.Schemas.MyPortfolio {
    static let balancesSample = balancesSample(values: [7_000_000_000, 2_700_000_000, 1_382_320_000])

    static let balancesSampleCashOnly = Self(
        cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
        pricesAsOf: nil, cabals: [])

    private static func balancesSample(values: [Int64]) -> Self {
        let cabals = [
            ("0f1d6a52-0001-7000-8000-000000000001", "Big Ballers"),
            ("0f1d6a52-0001-7000-8000-000000000002", "Tech Crew"),
            ("0f1d6a52-0001-7000-8000-000000000003", "Underwater"),
        ]
        let total = values.reduce(0, +)
        let rows = zip(cabals, values).map { cabal, value in
            Components.Schemas.PortfolioCabal(
                cabal: Components.Schemas.CabalRef(id: cabal.0, name: cabal.1, pictureUrl: nil),
                valueMicros: value, shareUnits: value, netContributedMicros: value, pnlMicros: 0, returnBps: nil,
                sliceBps: value * 10_000 / total, ownershipBps: 10_000, role: .admin, canVote: true,
                potValueMicros: value, memberCount: 1, openVoteCount: 0)
        }
        return Self(
            cashMicros: 0, netWorthMicros: total, totalValueMicros: total, pnlMicros: 0, returnBps: nil,
            computedAt: Date(timeIntervalSince1970: 1_790_996_460),
            pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400), cabals: rows)
    }
}

private struct BalancesPreviewTransport: ClientTransport {
    let scenario: BalancesModel.PreviewScenario

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch scenario {
        case .loading:
            try await Task.sleep(for: .seconds(3_600))
            throw CancellationError()
        case .failed:
            throw URLError(.notConnectedToInternet)
        case .ready:
            return try reply(request, portfolio: .balancesSample)
        case .cashOnly:
            return try reply(request, portfolio: .balancesSampleCashOnly)
        }
    }

    private func reply(_ request: HTTPRequest, portfolio: Components.Schemas.MyPortfolio) throws -> (
        HTTPResponse, HTTPBody?
    ) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let data: Data
        if request.path?.hasPrefix("/v1/me/balance") == true {
            data = try encoder.encode(
                Components.Schemas.Balance(
                    availableMicros: "500000000", onChainMicros: "500000000", inFlightMicros: "0",
                    depositAddress: "preview-wallet", asOf: Date(timeIntervalSince1970: 1_790_996_460)))
        } else {
            data = try encoder.encode(portfolio)
        }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(data))
    }
}

private struct BalancesPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct BalancesPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
