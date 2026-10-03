import MonacoAPI
import Observation

public struct AssetDetailPresentation: Equatable, Sendable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let kind: AssetKind
    public let priceMicros: Int64?
    public let changeBasisPoints: Int64?
    public let attribution: String
}

@Observable
@MainActor
public final class AssetDetailClientModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed(APIError)
    }

    public private(set) var phase: Phase = .idle
    public private(set) var detail: AssetDetailPresentation?
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private let symbol: String

    public init(api: APIClient, symbol: String) {
        self.api = api
        self.symbol = symbol
    }

    public func load() async {
        if detail == nil { phase = .loading }
        do {
            let response = try await api.read { client in
                try await client.getAsset(path: .init(symbol: symbol)).ok.body.json
            }
            detail = Self.presentation(response)
            phase = .loaded
            lastError = nil
        } catch {
            let error = APIError(error)
            lastError = error
            failureTick += 1
            if detail == nil { phase = .failed(error) }
        }
    }

    private static func presentation(_ detail: Components.Schemas.AssetDetail) -> AssetDetailPresentation {
        let kind: AssetKind = detail.kind == .equity ? .stock : .preIpo
        return AssetDetailPresentation(
            symbol: detail.symbol,
            ticker: AssetSymbolFormatter.display(detail.symbol, kind: kind),
            name: CatalogAssetNameFormatter.format(detail.displayName, kind: kind),
            kind: kind,
            priceMicros: detail.priceMicros,
            changeBasisPoints: detail.changeBps.map(Int64.init),
            attribution: detail.attribution
        )
    }
}
