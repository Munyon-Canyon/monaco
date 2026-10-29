import Foundation
import MonacoAPI
import Observation

public struct CabalStarter: Equatable, Sendable, Identifiable {
    public let asset: MarketAsset
    public let subline: String

    public var id: String { asset.symbol }
    public var isEligible: Bool { asset.kind == .preIpo }

    public static func subline(kind: AssetKind, ticker: String, cabalCount: Int?) -> String {
        switch (kind, cabalCount) {
        case (.preIpo, let count?): "Secondary estimate \u{00B7} in \(count) cabals"
        case (.preIpo, nil): "Secondary estimate"
        case (_, let count?): "In \(count) cabals"
        case (_, nil): ticker
        }
    }

    public init(_ summary: Components.Schemas.AssetSummary) {
        let asset = MarketMapping.asset(summary)
        self.asset = asset
        subline = Self.subline(kind: asset.kind, ticker: asset.ticker, cabalCount: summary.cabalCount.map { Int($0) })
    }
}

@Observable
@MainActor
public final class CabalStartersModel {
    public static let title = "Cabals like yours start with"
    public static let eligible = "Eligible"

    public private(set) var rows: [CabalStarter] = []

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public func load() async {
        guard
            let list = try? await api.read({ client in
                try await client.getAssets(query: .init(filter: .inCabals, limit: 5)).ok.body.json
            })
        else { return }
        rows = list.assets.map(CabalStarter.init)
    }

    public func observe() async {
        await refresher.observe([hints.hints(matching: .global(what: "prices_updated"))])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
