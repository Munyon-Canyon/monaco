#if DEBUG
import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

extension PortfolioModel {
    public static func preview(_ portfolio: Components.Schemas.MyPortfolio) -> PortfolioModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let api = APIClient(
            serverURL: serverURL, tokens: PortfolioPreviewTokens(),
            transport: PortfolioPreviewTransport(portfolio: portfolio))
        return PortfolioModel(api: api, hints: PortfolioPreviewHints())
    }
}

extension Components.Schemas.MyPortfolio {
    public static let sampleF = Components.Schemas.MyPortfolio(
        cashMicros: 500_000_000, netWorthMicros: 12_082_000_000, totalValueMicros: 11_582_000_000,
        pnlMicros: 241_100_000, returnBps: 203, computedAt: nil, pricesAsOf: nil, cabals: [])
}

extension PortfolioModel {
    public static var previewF: PortfolioModel { preview(.sampleF) }
}

private struct PortfolioPreviewTransport: ClientTransport {
    let portfolio: Components.Schemas.MyPortfolio

    func send(
        _: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(portfolio)))
    }
}

private struct PortfolioPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct PortfolioPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
