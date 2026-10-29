import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class ShareScreenModel {
    public static let pageSize = 100
    public static let maxPages = 20

    public let subject: ShareSubject
    public var format: ShareFormat
    public var eyes: ShareEyes {
        didSet { if eyes != oldValue { rebuild() } }
    }
    public private(set) var state: LoadState<ShareCardSpec> = .idle
    public private(set) var shareURL: URL?

    private let api: APIClient
    private let now: @Sendable () -> Date
    private let timeZone: TimeZone
    private let maxPages: Int
    private var loaded: Loaded?
    private var loadedKind: CardKind?
    private var generation = 0

    public init(
        subject: ShareSubject, api: APIClient, now: @escaping @Sendable () -> Date,
        timeZone: TimeZone = .current, maxPages: Int = ShareScreenModel.maxPages
    ) {
        self.subject = subject
        let defaults = Self.defaults(for: subject)
        eyes = defaults.eyes
        format = defaults.format
        self.api = api
        self.now = now
        self.timeZone = timeZone
        self.maxPages = maxPages
    }

    public var spec: ShareCardSpec? {
        if case .loaded(let spec) = state { return spec }
        return nil
    }

    public var title: String {
        switch loadedKind {
        case .vote: Self.voteTitle
        case .passed, .position: Self.positionTitle
        case nil:
            if case .openVote = subject { Self.voteTitle } else { Self.positionTitle }
        }
    }

    public var isVote: Bool { title == Self.voteTitle }

    public static let positionTitle = "Share"
    public static let voteTitle = "Share vote"

    public static func defaults(for subject: ShareSubject) -> (eyes: ShareEyes, format: ShareFormat) {
        switch subject {
        case .passedProposal: (ShareEyes(pnlDollars: false), .square)
        case .openVote: (voteDefaults, .square)
        default: (ShareEyes(), .post)
        }
    }

    private static let voteDefaults = ShareEyes(pnlDollars: false, buySell: false)

    public func load() async {
        switch subject {
        case .position(let cabalID, let symbol):
            await run { try await self.loadPosition(cabalID: cabalID, symbol: symbol) }
        case .passedProposal(let id), .openVote(let id):
            await run { try await self.loadProposal(id: id) }
        case .stake, .portfolio:
            return
        }
    }

    private func run(_ work: () async throws -> Loaded) async {
        generation += 1
        let issued = generation
        state = .loading
        do {
            let loaded = try await work()
            guard issued == generation, !Task.isCancelled else { return }
            apply(loaded)
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            state = .failed(APIError(error))
        }
    }

    private func apply(_ loaded: Loaded) {
        self.loaded = loaded
        loadedKind = loaded.kind
        shareURL = loaded.shareURL
        if loaded.kind != .position {
            self.eyes = loaded.kind == .vote ? Self.voteDefaults : Self.defaults(for: .passedProposal(id: "")).eyes
            format = .square
        }
        rebuild()
    }

    private func rebuild() {
        guard let loaded else { return }
        switch loaded.content {
        case .position(let position, let trades, let truncated):
            state = .loaded(
                SharePositionBuilder.build(
                    position: position, trades: trades, chart: loaded.chart, code: loaded.code, eyes: eyes,
                    truncated: truncated, now: now(), timeZone: timeZone))
        case .passed(let proposal, let photo):
            state = .loaded(
                SharePassedBuilder.build(
                    proposal: proposal, crest: loaded.crest, proposerPhoto: photo, chart: loaded.chart,
                    code: loaded.code, eyes: eyes, timeZone: timeZone))
        case .vote(let proposal):
            state = .loaded(
                ShareVoteBuilder.build(
                    proposal: proposal, crest: loaded.crest, chart: loaded.chart, code: loaded.code, eyes: eyes,
                    now: now(), timeZone: timeZone))
        }
    }

    private enum CardKind {
        case position
        case passed
        case vote
    }

    private enum Content: Sendable {
        case position(SharePosition, trades: [ShareTrade], truncated: Bool)
        case passed(Components.Schemas.ProposalDetail, photo: URL?)
        case vote(Components.Schemas.ProposalDetail)
    }

    private struct Loaded: Sendable {
        let content: Content
        let crest: ShareCrest
        let chart: [ShareChartPoint]
        let code: String?
        let shareURL: URL?

        var kind: CardKind {
            switch content {
            case .position: .position
            case .passed: .passed
            case .vote: .vote
            }
        }
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
            content: .position(position, trades: tradeValue.items, truncated: tradeValue.truncated),
            crest: position.crest, chart: chart, code: link?.value.uppercased(),
            shareURL: link.map { ReferralLink.url(for: $0, cabalID: cabalID) })
    }

    private func loadProposal(id: String) async throws -> Loaded {
        let api = api
        let proposal = try await api.read { try await $0.getProposal(path: .init(id: id)).ok.body.json }
        let isOpen = proposal.status == .open
        guard isOpen || SharePassedBuilder.isPassed(proposal.status) else {
            throw APIError.decoding("nothing to share for a \(proposal.status.rawValue) proposal")
        }
        let (cabalID, symbol, now) = (proposal.cabalId, proposal.symbol, now())
        async let cabal = api.read { try await $0.getCabal(path: .init(id: cabalID)).ok.body.json }
        async let referral = try? api.read { try await $0.getMyReferralCode().ok.body.json }
        let chart = try await Self.fetchChart(
            api: api, symbol: symbol, since: isOpen ? now : proposal.createdAt, now: now)
        let cabalValue = try await cabal
        let link = await referral.flatMap(Self.link)
        let crest = ShareCrest(
            cabalID: cabalID, name: cabalValue.name, pictureURL: cabalValue.pictureUrl.flatMap(URL.init(string:)))
        if isOpen {
            return Loaded(
                content: .vote(proposal), crest: crest, chart: chart, code: link?.value.uppercased(),
                shareURL: link.map { ReferralLink.url(for: $0, cabalID: cabalID, proposalID: id) })
        }
        return Loaded(
            content: .passed(proposal, photo: proposal.proposer.photoUrl.flatMap(URL.init(string:))), crest: crest,
            chart: chart, code: link?.value.uppercased(),
            shareURL: link.map { ReferralLink.url(for: $0, cabalID: cabalID) })
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
