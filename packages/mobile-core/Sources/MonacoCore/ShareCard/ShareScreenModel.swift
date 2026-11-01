import Foundation
import MonacoAPI
import MonacoRanking
import Observation

@Observable
@MainActor
public final class ShareScreenModel {
    public static let pageSize = 100
    public static let maxPages = 20

    public let subject: ShareSubject
    public var format: ShareFormat {
        didSet { formatChanged(from: oldValue) }
    }
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
    private var eyesBeforeSticker: ShareEyes?

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
        if format == .sticker { return Self.stickerTitle }
        return switch loadedKind {
        case .vote: Self.voteTitle
        case .passed, .position, .stake, .portfolio: Self.positionTitle
        case .ledger: ledgerTitle
        case nil:
            switch subject {
            case .openVote: Self.voteTitle
            case .ledger: ledgerTitle
            default: Self.positionTitle
            }
        }
    }

    public var isVote: Bool { loadedKind == .vote || (loadedKind == nil && isOpenVoteSubject) }

    private var isOpenVoteSubject: Bool {
        if case .openVote = subject { return true }
        return false
    }

    public static let positionTitle = "Share"
    public static let voteTitle = "Share vote"
    public static let stickerTitle = "Sticker"
    public static let stickerEyes = ShareEyes(pnlDollars: false, buySell: false, code: false)

    private func formatChanged(from old: ShareFormat) {
        if format == .sticker, old != .sticker {
            eyesBeforeSticker = eyes
            eyes = Self.stickerEyes
        } else if old == .sticker, format != .sticker, let saved = eyesBeforeSticker {
            eyesBeforeSticker = nil
            eyes = saved
        }
    }

    public static func defaults(for subject: ShareSubject) -> (eyes: ShareEyes, format: ShareFormat) {
        switch subject {
        case .passedProposal: (ShareEyes(pnlDollars: false), .square)
        case .openVote: (voteDefaults, .square)
        case .portfolio: (ShareEyes(), .post)
        case .ledger: (ShareEyes(), .story)
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
        case .stake(let cabalID):
            await run { try await self.loadStake(cabalID: cabalID) }
        case .portfolio:
            await run { try await self.loadPortfolio() }
        case .ledger(let week):
            await run { try await self.loadLedger(week: week) }
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
        switch loaded.kind {
        case .position, .stake, .portfolio, .ledger: break
        case .vote, .passed:
            if format != .sticker {
                self.eyes = loaded.kind == .vote ? Self.voteDefaults : Self.defaults(for: .passedProposal(id: "")).eyes
                format = .square
            }
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
        case .stake(let stake):
            state = .loaded(
                ShareStakeBuilder.build(
                    stake: stake, chart: loaded.chart, code: loaded.code, eyes: eyes, timeZone: timeZone))
        case .portfolio(let portfolio):
            state = .loaded(
                SharePortfolioBuilder.build(
                    portfolio: portfolio, history: loaded.chart, code: loaded.code, eyes: eyes, now: now(),
                    timeZone: timeZone))
        case .ledger(let content):
            let shown = format == .sticker ? ShareEyes(pnlDollars: true, buySell: false, code: false) : eyes
            state = .loaded(ShareLedgerBuilder.build(content: content, eyes: shown))
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
        case stake
        case portfolio
        case ledger
    }

    private enum Content: Sendable {
        case position(SharePosition, trades: [ShareTrade], truncated: Bool)
        case passed(Components.Schemas.ProposalDetail, photo: URL?)
        case vote(Components.Schemas.ProposalDetail)
        case stake(ShareStake)
        case portfolio(SharePortfolio)
        case ledger(LedgerShareContent)
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
            case .stake: .stake
            case .portfolio: .portfolio
            case .ledger: .ledger
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
        async let referral = Self.fetchReferral(api: api)
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
        guard let symbol = proposal.symbol else {
            throw APIError.decoding("nothing to share for a rule proposal")
        }
        let (cabalID, now) = (proposal.cabalId, now())
        async let cabal = api.read { try await $0.getCabal(path: .init(id: cabalID)).ok.body.json }
        async let referral = Self.fetchReferral(api: api)
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
}

extension ShareScreenModel {
    fileprivate var ledgerTitle: String {
        if case .ledger(let content)? = loaded?.content { return content.weekTitle }
        if case .ledger(let week) = subject { return LedgerShareContent.weekTitle(week) }
        return Self.positionTitle
    }

    private func loadStake(cabalID: String) async throws -> Loaded {
        let api = api
        async let pot = api.read { try await $0.getCabalPot(path: .init(id: cabalID)).ok.body.json }
        async let cabal = api.read { try await $0.getCabal(path: .init(id: cabalID)).ok.body.json }
        async let me = api.read { try await $0.getMe().ok.body.json }
        async let history = api.read {
            try await $0.getMyPnlHistory(query: .init(range: .all, cabalId: cabalID)).ok.body.json
        }
        async let referral = Self.fetchReferral(api: api)
        guard let slice = try await pot.me else { throw APIError.decoding("not a member of \(cabalID)") }
        let cabalValue = try await cabal
        let user = try await me
        let curve = try await history
        let link = await referral.flatMap(Self.link)
        let crest = ShareCrest(
            cabalID: cabalID, name: cabalValue.name, pictureURL: cabalValue.pictureUrl.flatMap(URL.init(string:)))
        let stake = ShareStake(
            crest: crest, name: user.displayName, photoURL: user.photoUrl.flatMap(URL.init(string:)),
            ownershipBps: Int64(slice.sliceBps), valueMicros: slice.valueMicros,
            netContributedMicros: slice.netContributedMicros, pnlMicros: slice.pnlMicros,
            firstFundedAt: slice.firstFundedAt)
        return Loaded(
            content: .stake(stake), crest: crest,
            chart: curve.points.map { ShareChartPoint(time: $0.at, priceMicros: $0.equityMicros) },
            code: link?.value.uppercased(), shareURL: link.map { ReferralLink.url(for: $0, cabalID: cabalID) })
    }

    private func loadLedger(week: String) async throws -> Loaded {
        let (api, timeZone) = (api, timeZone)
        async let ledger = api.read { try await $0.getMyLedger(path: .init(week: week)).ok.body.json }
        async let me = api.read { try await $0.getMe().ok.body.json }
        async let referral = Self.fetchReferral(api: api)
        let user = try await me
        let link = await referral.flatMap(Self.link)
        let name = user.displayName.split(separator: " ").first.map(String.init) ?? user.displayName
        let content = LedgerShareContent(
            try await ledger, name: name, photoURL: user.photoUrl.flatMap(URL.init(string:)),
            code: link?.value, timeZone: timeZone)
        return Loaded(
            content: .ledger(content), crest: ShareCrest(cabalID: "", name: "", pictureURL: nil), chart: [],
            code: content.code, shareURL: link.map { ReferralLink.url(for: $0) })
    }

    private func loadPortfolio() async throws -> Loaded {
        let api = api
        async let portfolio = api.read { try await $0.getMyPortfolio().ok.body.json }
        async let history = api.read { try await $0.getMyPnlHistory(query: .init(range: ._1m)).ok.body.json }
        async let referral = Self.fetchReferral(api: api)
        let id = try await api.read { try await $0.getMe().ok.body.json }.id
        let user = try await api.read { try await $0.getUser(path: .init(id: id)).ok.body.json }
        let holdings = try await portfolio
        let curve = try await history
        let hasBoardFigure = user.pnl1mMicros != nil
        let pnl = user.pnl1mMicros ?? curve.pnlMicros
        let returnBps = hasBoardFigure ? user.return1mBps : Self.curveReturn(curve)
        let link = await referral.flatMap(Self.link)
        let value = SharePortfolio(
            name: user.displayName, photoURL: user.photoUrl.flatMap(URL.init(string:)),
            cabalCount: holdings.cabals.count, totalValueMicros: holdings.totalValueMicros, pnl1mMicros: pnl,
            return1mBps: returnBps, rightCalls: user.standing.map { Int($0.right) },
            resolvedCalls: user.standing.map { Int($0.resolved) })
        return Loaded(
            content: .portfolio(value), crest: ShareCrest(cabalID: "", name: "", pictureURL: nil),
            chart: curve.points.map { ShareChartPoint(time: $0.at, priceMicros: $0.pnlMicros) },
            code: link?.value.uppercased(), shareURL: link.map { ReferralLink.url(for: $0) })
    }
}

extension ShareScreenModel {
    private nonisolated static func fetchReferral(api: APIClient) async -> Components.Schemas.MyReferralCode? {
        for _ in 0..<2 {
            if let code = try? await api.read({ try await $0.getMyReferralCode().ok.body.json }) { return code }
            if Task.isCancelled { return nil }
        }
        return nil
    }

    private nonisolated static func curveReturn(_ curve: Components.Schemas.MyPnlHistory) -> Int64? {
        guard curve.points.count >= 2, curve.returnKnown ?? (curve.returnBps != 0) else { return nil }
        return curve.returnBps
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
