import MonacoAPI
import Observation

public struct CabalTile: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let shortName: String
    public let pictureURL: String?
    public let isFunded: Bool
    public let direction: PortfolioSummary.Direction
    public let returnText: String
    public let treasuryText: String
    public let ownershipText: String
    public let valueText: String
    public let valueMicros: Int64
    public let memberCount: Int32
    public let role: String
    public let canVote: Bool
    public let unreadCount: Int

    init(_ cabal: Components.Schemas.PortfolioCabal, unreadCount: Int) {
        let funded = cabal.valueMicros > 0
        id = cabal.cabal.id
        name = cabal.cabal.name
        shortName = CabalHeaderCopy.shortName(cabal.cabal.name)
        pictureURL = cabal.cabal.pictureUrl
        isFunded = funded
        direction = funded ? PortfolioSummary.Direction(signedMicros: cabal.pnlMicros) : .flat
        returnText = funded ? PortfolioSummary.percent(cabal.returnBps) : "0%"
        treasuryText = CabalsTabCopy.wholeDollars(micros: cabal.potValueMicros)
        ownershipText = PercentFormatter.format(
            basisPoints: Int64(cabal.ownershipBps), signed: false, fractionDigits: 0)
        valueText = UsdAmountFormatter.format(micros: cabal.valueMicros)
        valueMicros = cabal.valueMicros
        memberCount = cabal.memberCount
        role = cabal.role.rawValue
        canVote = cabal.canVote
        self.unreadCount = unreadCount
    }

    public var shareLine: String { "\(treasuryText) treasury · \(ownershipText) yours" }

    public var spokenReturn: String {
        guard isFunded else { return "not funded" }
        let spoken = returnText.replacingOccurrences(of: "%", with: " percent")
        switch direction {
        case .up: return "up \(spoken.dropFirst())"
        case .down: return "down \(spoken.dropFirst())"
        case .flat: return "flat"
        }
    }
}

public struct CabalInviteTile: Identifiable, Equatable, Sendable {
    public let invite: ReceivedCabalInvite
    public var id: String { invite.id }
    public var shortName: String { CabalHeaderCopy.shortName(invite.cabalName) }
}

public struct CabalsTabContent: Equatable, Sendable {
    public let tiles: [CabalTile]
    public let invites: [CabalInviteTile]

    public var isEmpty: Bool { tiles.isEmpty && invites.isEmpty }
    public var showsReorderHint: Bool { tiles.count >= 2 }

    public var countText: String {
        invites.isEmpty ? "\(tiles.count)" : "\(tiles.count) · \(invites.count) invited"
    }
}

public enum CabalsTabCopy {
    public static let reorderHint = "Hold a cabal for actions · drag to reorder"

    public static func unread(_ count: Int) -> String? {
        switch count {
        case ..<1: nil
        case 1: "1 unread message"
        default: "\(count) unread messages"
        }
    }

    public static func wholeDollars(micros: Int64) -> String {
        let dollars = (micros + 500_000) / 1_000_000
        return String(UsdAmountFormatter.format(micros: dollars * 1_000_000).dropLast(3))
    }
}

@Observable
@MainActor
public final class CabalsTabModel {
    public private(set) var state: LoadState<CabalsTabContent> = .idle
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private var generation = 0
    private var settled = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient) {
        self.api = api
    }

    public var content: CabalsTabContent? {
        if case .loaded(let content) = state { return content }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {} else { state = .loading }
        do {
            async let portfolio = api.read { try await $0.getMyPortfolio().ok.body.json }
            async let invites = api.read { try await $0.getMyCabalInvites().ok.body.json }
            async let cabals = api.read { try await $0.getMyCabals().ok.body.json }
            let portfolioCabals = try await portfolio.cabals
            let mine = (try? await cabals) ?? []
            let unread = Dictionary(mine.map { ($0.id, Int($0.unreadCount)) }, uniquingKeysWith: { first, _ in first })
            let tiles = portfolioCabals.map { CabalTile($0, unreadCount: unread[$0.cabal.id] ?? 0) }
            let pending = try await invites.map { CabalInviteTile(invite: ReceivedCabalInvite($0)) }
            guard issued > settled else { return }
            settled = issued
            state = .loaded(CabalsTabContent(tiles: tiles, invites: pending))
            lastError = nil
        } catch {
            if Task.isCancelled { return }
            guard issued > settled else { return }
            settled = issued
            let error = APIError(error)
            if BackgroundRefresh.isActive, case .loaded = state { return }
            lastError = error
            failureTick += 1
            if case .loaded = state { return }
            state = .failed(error)
        }
    }

    public func applyOrder(_ ids: [String]) {
        guard let content else { return }
        let byID = Dictionary(content.tiles.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let ordered = ids.compactMap { byID[$0] }
        let rest = content.tiles.filter { !ids.contains($0.id) }
        state = .loaded(CabalsTabContent(tiles: ordered + rest, invites: content.invites))
    }

    public func observe(hints: any HintSource) async {
        await refresher.observe(hints.hints(matching: .user(what: nil)))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}

@Observable
@MainActor
public final class CabalsTabStore {
    public let tab: CabalsTabModel
    public let order: CabalOrderModel
    public let votes: HomePendingVotesModel
    public let needs = HomeVoteNeeds()

    public init(api: APIClient, hints: any HintSource, portfolio: PortfolioModel?) {
        let tab = CabalsTabModel(api: api)
        self.tab = tab
        order = CabalOrderModel(
            api: api,
            saved: {
                await tab.load()
                await portfolio?.load()
            })
        votes = HomePendingVotesModel(api: api, hints: hints)
    }

    public var content: CabalsTabContent? { tab.content }

    public var tiles: [CabalTile] {
        guard let tiles = content?.tiles else { return [] }
        let byID = Dictionary(tiles.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let ordered = order.order.compactMap { byID[$0] }
        return ordered.count == tiles.count ? ordered : tiles
    }

    public var hasFailed: Bool {
        if case .failed = tab.state { return true }
        return false
    }

    public var totalWaiting: Int { needs.byCabal.values.reduce(0, +) }

    public func syncOrder() {
        order.sync(content?.tiles.map(\.id) ?? [])
    }

    public func syncVotes() {
        needs.update(rows: votes.rows, votedThisSession: votes.votedIDs)
    }
}
