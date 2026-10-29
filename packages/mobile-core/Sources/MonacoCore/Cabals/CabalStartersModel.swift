import Foundation
import MonacoAPI
import Observation

public struct CabalStarter: Equatable, Sendable, Identifiable {
    public let asset: MarketAsset
    public let subline: String
    public let priceMicros: Int64?
    public let changeBasisPoints: Int64?
    public let eligibility: String?

    public var id: String { asset.symbol }

    init(_ summary: Components.Schemas.AssetSummary) {
        let asset = MarketMapping.asset(summary)
        let isPreIpo = asset.kind == .preIpo
        self.asset = asset
        subline = CabalStartersModel.subline(
            ticker: asset.ticker, isPreIpo: isPreIpo, cabalCount: summary.cabalCount.map { Int($0) })
        priceMicros = isPreIpo ? asset.estimate?.priceMicros ?? asset.priceMicros : asset.priceMicros
        changeBasisPoints = isPreIpo ? nil : asset.changeBasisPoints
        eligibility = isPreIpo ? (asset.isTradable ? "Eligible" : "Research only") : nil
    }
}

@Observable
@MainActor
public final class CabalStartersModel {
    public static let title = "Cabals like yours start with"
    public static let limit = 5

    public private(set) var state: LoadState<[CabalStarter]> = .idle

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public var starters: [CabalStarter] {
        if case .loaded(let starters) = state { return starters }
        return []
    }

    public func load() async {
        if case .idle = state { state = .loading }
        do {
            let list = try await api.read { client in
                try await client.getAssets(query: .init(filter: .inCabals, limit: Self.limit)).ok.body.json
            }
            state = .loaded(list.assets.map(CabalStarter.init))
        } catch {
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "prices_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    nonisolated static func subline(ticker: String, isPreIpo: Bool, cabalCount: Int?) -> String {
        guard let cabalCount else { return isPreIpo ? "Secondary estimate" : ticker }
        if isPreIpo { return "Secondary estimate \u{00B7} in \(cabalCount) cabals" }
        return "In \(cabalCount) cabals"
    }
}
