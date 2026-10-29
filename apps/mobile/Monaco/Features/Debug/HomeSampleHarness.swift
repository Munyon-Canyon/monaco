#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeSampleScenario: String, CaseIterable {
    case populated
    case empty
    case loading
    case nudgeX
    case nudgeXEmpty
    case prePrompt
    case votes
    case loss
    case gain
    case cashOnly
    case notFunded
    case partlyFunded
    case multipleProposals
    case voted
    case executing
    case completed
    case expired
    case blocked
    case noProposals
    case proposalsLoading
    case proposalsError
    case stale
    case error
    case fVotePick
    case fVoteHolding
    case fVoteCommitted
    case fVoteRule

    static func matching(_ arguments: [String]) -> HomeSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoHomeSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return HomeSampleScenario(rawValue: arguments[flag + 1])
    }
}

extension HomeSampleScenario {
    var script: SampleAPIScript {
        switch self {
        case .populated: SampleAPIScript()
        case .empty: HomeSampleData.script(.empty, proposals: [])
        case .loading: SampleAPIScript(mode: .hang)
        case .nudgeX: SampleAPIScript()
        case .nudgeXEmpty: SampleAPIScript(mode: .empty)
        case .prePrompt: SampleAPIScript()
        case .votes:
            SampleAPIScript(
                proposal: .sample(canVote: true), pendingVotes: Components.Schemas.PendingVote.samples,
                viewerCanVote: true)
        case .loss: HomeSampleData.script(.loss, proposals: [.needsVote])
        case .gain: HomeSampleData.script(.gain, proposals: [.needsVote])
        case .cashOnly: HomeSampleData.script(.cashOnly, proposals: [])
        case .notFunded: HomeSampleData.script(.unfunded, proposals: [])
        case .partlyFunded: HomeSampleData.script(.partlyFunded, proposals: [])
        case .multipleProposals: HomeSampleData.script(.loss, proposals: [.needsVote, .second])
        case .voted: HomeSampleData.script(.loss, proposals: [.voted])
        case .executing: HomeSampleData.script(.loss, proposals: [.executing])
        case .completed: HomeSampleData.script(.loss, proposals: [.completed])
        case .expired: HomeSampleData.script(.loss, proposals: [.expired])
        case .blocked: HomeSampleData.script(.loss, proposals: [.blocked])
        case .noProposals: HomeSampleData.script(.loss, proposals: [])
        case .proposalsLoading: HomeSampleData.script(.loss, proposals: [.needsVote], pendingVotesHang: true)
        case .proposalsError: HomeSampleData.script(.loss, proposals: [], pendingVotesFail: true)
        case .stale: HomeSampleData.script(.loss, proposals: [.needsVote], portfolioReadsBeforeFailing: 1)
        case .error: SampleAPIScript(mode: .offline)
        case .fVotePick, .fVoteHolding, .fVoteCommitted: HomeSampleData.script(.loss, proposals: [.fVote])
        case .fVoteRule: HomeSampleData.script(.loss, proposals: [.fRule])
        }
    }

    var voteCardSetup: (@MainActor (HomeVoteCardModel) -> Void)? {
        switch self {
        case .fVotePick: { $0.preview(.picked(.yes)) }
        case .fVoteHolding: { $0.preview(.holding(.yes, progress: 0.4)) }
        case .fVoteCommitted:
            {
                let outcome = CastOutcome(
                    choice: .yes, status: .passed, tally: ProposalTally(yes: 5, no: 1, voters: 8, needed: 5),
                    undoUntil: nil,
                    myVote: ProposalMyVote(order: 5, of: 8, yesOrder: 5, noOrder: 0, decided: true))
                $0.preview(.committed(outcome), sharesEstimate: "1.82")
            }
        default: nil
        }
    }

    var hints: any HintConnecting {
        self == .stale ? SampleResyncHints(delay: .seconds(2)) : SampleSilentHints()
    }
}

enum HomeSampleData {
    enum Figures {
        case loss, gain, cashOnly, unfunded, partlyFunded, empty
    }

    enum Proposal {
        case needsVote, second, voted, executing, completed, expired, blocked, fVote, fRule
    }

    static let bigBallers = "0f1d6a52-0001-7000-8000-000000000001"
    static let techCrew = "0f1d6a52-0001-7000-8000-000000000002"
    static let underwater = "0f1d6a52-0001-7000-8000-000000000003"

    private static let cabals: [(id: String, name: String, members: Int32, ownershipBps: Int32)] = [
        (bigBallers, "Big Ballers", 8, 3_500), (techCrew, "Tech Crew", 5, 1_400), (underwater, "Underwater", 9, 650),
    ]

    @MainActor
    static func script(
        _ figures: Figures, proposals: [Proposal], pendingVotesHang: Bool = false, pendingVotesFail: Bool = false,
        portfolioReadsBeforeFailing: Int? = nil
    ) -> SampleAPIScript {
        var script = SampleAPIScript(mode: figures == .empty ? .empty : .populated)
        script.portfolio = portfolio(figures)
        script.pnlHistory = history(figures)
        script.balance = balance(micros: figures == .empty ? 0 : 500_000_000)
        script.homeCabals = figures == .empty ? [] : cabals.map(myCabal)
        script.ownershipBps = Dictionary(
            uniqueKeysWithValues: cabals.map { ($0.id, ownershipBps(figures, $0)) })
        script.cabalBoard = cabalBoard
        script.pendingVotes = proposals.map(pending)
        script.proposals = Dictionary(uniqueKeysWithValues: proposals.map { (pending($0).proposalId, detail($0)) })
        script.assets = [
            "NVDAx": asset("NVDAx", "NVIDIA", changeBps: 418), "MSFTx": asset("MSFTx", "Microsoft", changeBps: 96),
        ]
        script.catalog = [
            catalogEntry("NVDAx", "NVIDIA", changeBps: 418), catalogEntry("MSFTx", "Microsoft", changeBps: 96), .googl,
        ]
        script.pendingVotesHang = pendingVotesHang
        script.pendingVotesFail = pendingVotesFail
        script.portfolioReadsBeforeFailing = portfolioReadsBeforeFailing
        return script
    }

    private static func ownershipBps(
        _ figures: Figures, _ cabal: (id: String, name: String, members: Int32, ownershipBps: Int32)
    )
        -> Int32
    {
        switch figures {
        case .cashOnly, .unfunded: 0
        case .partlyFunded: cabal.id == bigBallers ? cabal.ownershipBps : 0
        case .loss, .gain, .empty: cabal.ownershipBps
        }
    }

    private static func balance(micros: Int64) -> Components.Schemas.Balance {
        Components.Schemas.Balance(
            availableMicros: "\(micros)", onChainMicros: "\(micros)", inFlightMicros: "0", depositAddress: "wallet-1",
            asOf: .now)
    }

    private static func portfolio(_ figures: Figures) -> Components.Schemas.MyPortfolio {
        let values: [Int64]
        let pnls: [Int64]
        switch figures {
        case .loss:
            values = [7_000_000_000, 2_782_320_000, 1_300_000_000]
            pnls = [-300_000_000, 50_000_000, -167_680_000]
        case .gain:
            values = [8_500_000_000, 3_182_320_000, 1_300_000_000]
            pnls = [1_100_000_000, 300_000_000, 82_320_000]
        case .cashOnly:
            values = [0, 0, 0]
            pnls = [0, 0, 0]
        case .partlyFunded:
            values = [7_000_000_000, 0, 0]
            pnls = [-300_000_000, 0, 0]
        case .unfunded, .empty:
            return .sampleEmpty
        }
        let total: Int64 = values.reduce(0, +)
        let totalPnl: Int64 = pnls.reduce(0, +)
        let totalNetContributed: Int64 = total - totalPnl
        let totalReturnBps: Int64? = total == 0 ? nil : totalPnl * 10_000 / max(totalNetContributed, 1)
        let paired = Array(zip(cabals, zip(values, pnls)))
        let shown = figures == .partlyFunded ? paired.filter { $0.1.0 > 0 } : paired
        let rows: [Components.Schemas.PortfolioCabal] = shown.map { cabal, figure in
            let value: Int64 = figure.0
            let pnl: Int64 = figure.1
            let netContributed: Int64 = value - pnl
            let returnBps: Int64? = value == 0 ? nil : pnl * 10_000 / max(netContributed, 1)
            let sliceBps: Int64 = total == 0 ? 0 : value * 10_000 / total
            return Components.Schemas.PortfolioCabal(
                cabal: Components.Schemas.CabalRef(id: cabal.id, name: cabal.name, pictureUrl: nil),
                valueMicros: value, shareUnits: value, netContributedMicros: netContributed, pnlMicros: pnl,
                returnBps: returnBps, sliceBps: sliceBps, ownershipBps: 10_000, role: .admin, canVote: true,
                potValueMicros: value, memberCount: 1, openVoteCount: 0)
        }
        return Components.Schemas.MyPortfolio(
            cashMicros: 0, netWorthMicros: total, totalValueMicros: total, pnlMicros: totalPnl,
            returnBps: totalReturnBps, computedAt: .now,
            pricesAsOf: .now, cabals: rows)
    }

    private static func history(_ figures: Figures) -> Components.Schemas.MyPnlHistory {
        let target: Int64
        switch figures {
        case .loss, .partlyFunded: target = -417_680_000
        case .gain: target = 1_482_320_000
        case .cashOnly, .unfunded, .empty: return .sampleEmpty
        }
        let count = 31
        let wobbles: [Int64] = [0, 18, -12, 25, -20, 10, -25, 15]
        let start = Date.now.addingTimeInterval(-30 * 86_400)
        let points = (0..<count).map { index -> Components.Schemas.PnlPoint in
            let last = Int64(count - 1)
            let wobble = wobbles[index % wobbles.count] * 1_000_000 * (last - Int64(index)) / last
            let pnl = target * Int64(index) / last + wobble
            return Components.Schemas.PnlPoint(
                at: start.addingTimeInterval(TimeInterval(index) * 86_400), equityMicros: 12_000_000_000 + pnl,
                pnlMicros: pnl)
        }
        return Components.Schemas.MyPnlHistory(
            range: ._1m, pnlMicros: target, returnBps: target * 10_000 / 12_000_000_000, points: points)
    }

    private static func myCabal(_ cabal: (id: String, name: String, members: Int32, ownershipBps: Int32))
        -> Components.Schemas.MyCabal
    {
        Components.Schemas.MyCabal(
            id: cabal.id, name: cabal.name, pictureUrl: nil, role: "member", canVote: true, memberCount: cabal.members,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000), pendingRequestCount: 0, unreadCount: 0)
    }

    private static var cabalBoard: [Components.Schemas.LeaderboardRow] {
        [
            row(1, "ape-society", "Ape Society", pnl: 12_690_000_000, bps: 4_210),
            row(2, "quant-kids", "Quant Kids", pnl: 7_420_000_000, bps: 3_710),
            row(3, "dip-buyers", "Dip Buyers", pnl: 3_180_000_000, bps: 1_960),
        ]
    }

    private static func row(_ rank: Int32, _ id: String, _ name: String, pnl: Int64, bps: Int64)
        -> Components.Schemas.LeaderboardRow
    {
        .sample(
            rank: rank, id: id, name: name, kind: .cabal, valueMicros: pnl * 3, pnlMicros: pnl, returnBps: bps)
    }

    private static func pending(_ proposal: Proposal) -> Components.Schemas.PendingVote {
        let second = proposal == .second
        if proposal == .fRule {
            var rule = Components.Schemas.PendingVote.ruleSample(now: .now)
            rule.proposalId = "home-proposal-1"
            rule.cabalId = bigBallers
            rule.cabalName = "Big Ballers"
            return rule
        }
        let closes: TimeInterval = proposal == .fVote ? 4 * 3_600 + 600 : 8 * 3_600
        return Components.Schemas.PendingVote.sample(
            proposalId: "home-proposal-\(second ? 2 : 1)", cabalId: second ? techCrew : bigBallers,
            cabalName: second ? "Tech Crew" : "Big Ballers", symbol: second ? "MSFTx" : "NVDAx",
            assetName: second ? "Microsoft" : "NVIDIA", expiresAt: .now.addingTimeInterval(closes))
    }

    private static func detail(_ proposal: Proposal) -> Components.Schemas.ProposalDetail {
        var detail = Components.Schemas.ProposalDetail.sample(
            status: status(proposal), myBallot: proposal == .voted ? .yes : nil, canVote: true)
        let second = proposal == .second
        detail.cabalId = second ? techCrew : bigBallers
        detail.cabalName = second ? "Tech Crew" : "Big Ballers"
        detail.symbol = second ? "MSFTx" : "NVDAx"
        detail.usdcMicros = second ? 250_000_000 : 400_000_000
        detail.tally = .init(yes: 3, no: 1, voters: 8, needed: 5)
        return detail
    }

    private static func status(_ proposal: Proposal) -> Components.Schemas.ProposalStatus {
        switch proposal {
        case .needsVote, .second, .voted, .fVote, .fRule: .open
        case .executing: .passed
        case .completed: .executed
        case .expired: .expired
        case .blocked: .executionBlocked
        }
    }

    private static func catalogEntry(
        _ symbol: String, _ name: String, changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        var entry = Components.Schemas.AssetSummary.googl
        entry.symbol = symbol
        entry.displayName = name
        entry.logoUrl = nil
        entry.changeBps = changeBps
        return entry
    }

    private static func asset(_ symbol: String, _ name: String, changeBps: Int32) -> Components.Schemas.AssetDetail {
        var asset = Components.Schemas.AssetDetail.googl
        asset.symbol = symbol
        asset.displayName = name
        asset.logoUrl = nil
        asset.changeBps = changeBps
        return asset
    }
}

private struct SampleNotificationAuthorizing: NotificationAuthorizing {
    func status() async -> PushAuthorization { .notDetermined }
    func request() async -> Bool { false }
}

private enum SamplePushPrePrompt {
    @MainActor static func make() -> PushPrePrompt {
        PushPrePrompt(
            authorization: SampleNotificationAuthorizing(),
            defaults: UserDefaults(suiteName: "sample-push-pre-prompt") ?? .standard,
            register: {})
    }
}

struct HomeSampleHarness: View {
    let scenario: HomeSampleScenario
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(auth: auth, tab: .home, session: session, sheet: sheet, hints: scenario.hints)
    }

    private var sheet: (() -> AnyView)? {
        guard scenario == .prePrompt else { return nil }
        return { AnyView(PushPrePromptSheet(prompt: SamplePushPrePrompt.make())) }
    }

    private func session(_ store: AppSessionStore) {
        if scenario == .loading {
            SampleAppFrame.loading(store)
        } else {
            SampleAppFrame.signedIn(store)
            let connectX = ProcessInfo.processInfo.arguments.contains("-MonacoFeatureConnectX")
            if connectX && (scenario == .nudgeX || scenario == .nudgeXEmpty) {
                store.profile?.authState = .awaitingSocials
            }
        }
    }
}

final class HomeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = HomeSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(HomeSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
