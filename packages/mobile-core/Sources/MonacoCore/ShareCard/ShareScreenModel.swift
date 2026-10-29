import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class ShareScreenModel {
    public static let pageSize = 100
    public static let maxPages = 20

    public let subject: ShareSubject
    public var format: ShareFormat = .post
    public var eyes = ShareEyes() {
        didSet { if eyes != oldValue { rebuild() } }
    }
    public private(set) var state: LoadState<ShareCardSpec> = .idle
    public private(set) var shareURL: URL?

    private let api: APIClient
    private let now: @Sendable () -> Date
    private let timeZone: TimeZone
    private let maxPages: Int
    private var loaded: Loaded?
    private var generation = 0

    public init(
        subject: ShareSubject, api: APIClient, now: @escaping @Sendable () -> Date,
        timeZone: TimeZone = .current, maxPages: Int = ShareScreenModel.maxPages
    ) {
        self.subject = subject
        self.api = api
        self.now = now
        self.timeZone = timeZone
        self.maxPages = maxPages
    }

    public var spec: ShareCardSpec? {
        if case .loaded(let spec) = state { return spec }
        return nil
    }

    public func load() async {
        guard case .position(let cabalID, let symbol) = subject else { return }
        generation += 1
        let issued = generation
        state = .loading
        do {
            let loaded = try await loadPosition(cabalID: cabalID, symbol: symbol)
            guard issued == generation, !Task.isCancelled else { return }
            self.loaded = loaded
            shareURL = loaded.shareURL
            rebuild()
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            state = .failed(APIError(error))
        }
    }

    private func rebuild() {
        guard let loaded else { return }
        state = .loaded(
            SharePositionBuilder.build(
                position: loaded.position, trades: loaded.trades, chart: loaded.chart, code: loaded.code, eyes: eyes,
                truncated: loaded.truncated, now: now(), timeZone: timeZone))
    }

    private struct Loaded: Sendable {
        let position: SharePosition
        let trades: [ShareTrade]
        let chart: [ShareChartPoint]
        let code: String?
        let shareURL: URL?
        let truncated: Bool
    }

    private struct Trades: Sendable {
        let items: [ShareTrade]
        let truncated: Bool
    }

    private func loadPosition(cabalID: String, symbol: String) async throws -> Loaded {
        let (api, maxPages) = (api, maxPages)
        async let pot = api.read { try await $0.getCabalPot(path: .init(id: cabalID)).ok.body.json }
        async let cabal = api.read { try await $0.getCabal(path: .init(id: cabalID)).ok.body.json }
        async let trades = Self.fetchTrades(api: api, cabalID: cabalID, symbol: symbol, maxPages: maxPages)
        async let referral = try? api.read { try await $0.getMyReferralCode().ok.body.json }
        let potValue = try await pot
        guard let holding = potValue.holdings.first(where: { $0.symbol == symbol }) else {
            throw APIError.decoding("no position in \(symbol)")
        }
        let chart = try await Self.fetchChart(api: api, symbol: symbol, since: holding.firstBoughtAt, now: now())
        let cabalValue = try await cabal
        let tradeValue = try await trades
        let link = await referral.flatMap(Self.link)
        let position = SharePosition(
            crest: ShareCrest(
                cabalID: cabalID, name: cabalValue.name, pictureURL: cabalValue.pictureUrl.flatMap(URL.init(string:))),
            symbol: AssetSymbolFormatter.display(symbol, kind: AssetKind(raw: holding.kind.rawValue)),
            logoURL: nil, unitsText: holding.units, valueMicros: holding.valueMicros,
            costBasisMicros: holding.costBasisMicros, pnlMicros: holding.pnlMicros,
            avgEntryMicros: holding.avgEntryMicros.flatMap { Int64($0) }, priceMicros: holding.priceMicros,
            firstBoughtAt: holding.firstBoughtAt)
        return Loaded(
            position: position, trades: tradeValue.items, chart: chart, code: link?.value.uppercased(),
            shareURL: link.map { ReferralLink.url(for: $0, cabalID: cabalID) }, truncated: tradeValue.truncated)
    }

    private nonisolated static func link(_ code: Components.Schemas.MyReferralCode) -> ReferralCode? {
        if code.handleUnlocked, let handle = code.handleLink.flatMap(ReferralLink.parse) { return handle }
        return ReferralCode(code.code)
    }

    private nonisolated static func fetchTrades(
        api: APIClient, cabalID: String, symbol: String, maxPages: Int
    ) async throws -> Trades {
        var items: [ShareTrade] = []
        var cursor: String?
        for _ in 0..<maxPages {
            let page = try await api.read { [cursor] client in
                try await client.getCabalActivity(
                    path: .init(id: cabalID), query: .init(limit: pageSize, cursor: cursor, symbol: symbol)
                ).ok.body.json
            }
            items += page.items.compactMap(trade)
            cursor = page.nextCursor
            if cursor == nil { return Trades(items: items, truncated: false) }
        }
        return Trades(items: items, truncated: true)
    }

    private nonisolated static func trade(_ activity: Components.Schemas.CabalActivity) -> ShareTrade? {
        guard activity.status == .confirmed, activity.kind == .buy || activity.kind == .sell,
            let price = activity.priceMicros
        else { return nil }
        return ShareTrade(isBuy: activity.kind == .buy, time: activity.occurredAt, priceMicros: price)
    }

    private nonisolated static func fetchChart(
        api: APIClient, symbol: String, since: Date?, now: Date
    ) async throws -> [ShareChartPoint] {
        let range = range(since: since, now: now)
        let chart = try await api.read { client in
            try await client.getAssetChart(path: .init(symbol: symbol), query: .init(range: range)).ok.body.json
        }
        return chart.points.map { ShareChartPoint(time: $0.t, priceMicros: $0.closeMicros) }
    }

    nonisolated static func range(since: Date?, now: Date) -> Operations.GetAssetChart.Input.Query.RangePayload {
        guard let since else { return .all }
        let days = now.timeIntervalSince(since) / 86_400
        switch days {
        case ..<1: return ._1d
        case ..<7: return ._1w
        case ..<30: return ._1m
        case ..<90: return ._3m
        case ..<180: return ._6m
        case ..<365: return ._1y
        default: return .all
        }
    }
}
