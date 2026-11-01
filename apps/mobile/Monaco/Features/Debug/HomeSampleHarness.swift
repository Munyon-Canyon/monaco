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
    case notFunded
    case partlyFunded
    case staleStake
    case settlingStake
    case fVotePick
    case fVoteHolding
    case fVoteCommitted
    case fVoteRule
    case fReady
    case fReadyLight
    case fScrolled
    case feedPreviewEveryone
    case fUnfunded
    case fFirstUse

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
        case .populated: HomeSampleData.feedPreviewScript(.yourCabals)
        case .feedPreviewEveryone: HomeSampleData.feedPreviewScript(.everyone)
        case .empty: HomeSampleData.script(.empty, proposals: [])
        case .loading: SampleAPIScript(mode: .hang)
        case .nudgeX: SampleAPIScript()
        case .nudgeXEmpty: SampleAPIScript(mode: .empty)
        case .prePrompt: SampleAPIScript()
        case .votes:
            SampleAPIScript(
                proposal: .sample(canVote: true), pendingVotes: Components.Schemas.PendingVote.samples,
                viewerCanVote: true)
        case .notFunded: HomeSampleData.script(.unfunded, proposals: [])
        case .partlyFunded: HomeSampleData.script(.partlyFunded, proposals: [])
        case .staleStake: HomeSampleData.stale(HomeSampleData.script(.loss, proposals: []))
        case .settlingStake: HomeSampleData.settling(HomeSampleData.script(.loss, proposals: []))
        case .fVotePick, .fVoteHolding, .fVoteCommitted: HomeSampleData.script(.loss, proposals: [.fVote])
        case .fVoteRule: HomeSampleData.script(.loss, proposals: [.fRule])
        case .fReady, .fReadyLight, .fScrolled: HomeSampleData.script(.fAccount, proposals: [.fVote])
        case .fUnfunded: HomeSampleData.script(.fUnfunded, proposals: [.fVote])
        case .fFirstUse: HomeSampleData.firstUseScript()
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
        SampleSilentHints()
    }
}

enum HomeSampleData {
    enum Figures {
        case loss, unfunded, partlyFunded, empty, fAccount, fUnfunded
    }

    enum Proposal {
        case needsVote, voted, fVote, fRule
    }

    static let bigBallers = "0f1d6a52-0001-7000-8000-000000000001"
    static let techCrew = "0f1d6a52-0001-7000-8000-000000000002"
    static let underwater = "0f1d6a52-0001-7000-8000-000000000003"

    private static let cabals: [(id: String, name: String, members: Int32, ownershipBps: Int32)] = [
        (bigBallers, "Big Ballers", 8, 3_500), (techCrew, "Tech Crew", 5, 1_400), (underwater, "Underwater", 9, 650),
    ]

    static func stale(_ script: SampleAPIScript) -> SampleAPIScript {
        var script = script
        script.portfolio?.cabals[1].stale = true
        script.portfolio?.cabals[1].valuedAt = Date.now.addingTimeInterval(-3 * 86_400)
        return script
    }

    static func settling(_ script: SampleAPIScript) -> SampleAPIScript {
        var script = script
        script.portfolio?.cabals[1].valueMicros = 0
        script.portfolio?.cabals[1].pnlMicros = 0
        script.portfolio?.cabals[1].returnBps = nil
        script.portfolio?.cabals[1].arrivingMicros = 100_000_000
        return script
    }

    static func feedPreviewScript(_ source: HomeFeedPreviewModel.Source) -> SampleAPIScript {
        var script = SampleAPIScript()
        script.homeFeed = feedPreviewItems(source)
        return script
    }

    private static func feedPreviewItems(_ source: HomeFeedPreviewModel.Source) -> [Components.Schemas.FeedItem] {
        typealias Item = Components.Schemas.FeedItem
        var proposal = source == .yourCabals ? Item.samples[0] : Item.publicProposals[0]
        proposal.symbol = source == .yourCabals ? "NVDAx" : "AMZNx"
        proposal.action = "buy"
        proposal.usdcMicros = source == .yourCabals ? 400_000_000 : 250_000_000
        proposal.cabalName = source == .yourCabals ? "AC Capital" : "Degens"
        proposal.actorName = source == .yourCabals ? "Mia" : "Christelle"
        proposal.createdAt = Date.now.addingTimeInterval(source == .yourCabals ? -300 : -3_600)
        guard source == .yourCabals else {
            var stripe = Item.publicProposals[1]
            stripe.action = "buy"
            stripe.usdcMicros = 300_000_000
            return [proposal, Item.results[1], stripe, Item.results[2]]
        }
        return [proposal, Item.results[1], Item.results[0], Item.samples[4], Item.results[4]]
    }

    @MainActor
    static func script(
        _ figures: Figures, proposals: [Proposal]
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
        if figures == .fAccount || figures == .fUnfunded { applyF(&script, unfunded: figures == .fUnfunded) }
        return script
    }

    private static func ownershipBps(
        _ figures: Figures, _ cabal: (id: String, name: String, members: Int32, ownershipBps: Int32)
    )
        -> Int32
    {
        switch figures {
        case .unfunded: 0
        case .partlyFunded: cabal.id == bigBallers ? cabal.ownershipBps : 0
        case .loss, .empty, .fAccount, .fUnfunded: cabal.ownershipBps
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
        case .partlyFunded:
            values = [7_000_000_000, 0, 0]
            pnls = [-300_000_000, 0, 0]
        case .fAccount, .fUnfunded:
            return .sampleF
        case .unfunded:
            values = [0, 0, 0]
            pnls = [0, 0, 0]
        case .empty:
            return .sampleEmpty
        }
        let total: Int64 = values.reduce(0, +)
        let totalPnl: Int64 = pnls.reduce(0, +)
        let totalNetContributed: Int64 = total - totalPnl
        let totalReturnBps: Int64? = total == 0 ? nil : totalPnl * 10_000 / max(totalNetContributed, 1)
        let paired = Array(zip(cabals, zip(values, pnls)))
        let rows: [Components.Schemas.PortfolioCabal] = paired.map { cabal, figure in
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
        case .fAccount, .fUnfunded: target = 241_100_000
        case .unfunded, .empty: return .sampleEmpty
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
            id: cabal.id, name: cabal.name, pictureUrl: nil,
            status: "active", role: "member", canVote: true, memberCount: cabal.members,
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
        if proposal == .fRule {
            var rule = Components.Schemas.PendingVote.ruleSample(now: .now)
            rule.proposalId = "home-proposal-1"
            rule.cabalId = bigBallers
            rule.cabalName = "Big Ballers"
            return rule
        }
        let closes: TimeInterval = proposal == .fVote ? 4 * 3_600 + 600 : 8 * 3_600
        return Components.Schemas.PendingVote.sample(
            proposalId: "home-proposal-1", cabalId: bigBallers, cabalName: "Big Ballers", symbol: "NVDAx",
            assetName: "NVIDIA", expiresAt: .now.addingTimeInterval(closes))
    }

    private static func detail(_ proposal: Proposal) -> Components.Schemas.ProposalDetail {
        var detail = Components.Schemas.ProposalDetail.sample(
            status: .open, myBallot: proposal == .voted ? .yes : nil, canVote: true)
        detail.cabalId = bigBallers
        detail.cabalName = "Big Ballers"
        detail.symbol = "NVDAx"
        detail.usdcMicros = 400_000_000
        detail.tally = .init(yes: 3, no: 1, voters: 8, needed: 5)
        return detail
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

extension HomeSampleData {
    static func firstUseScript() -> SampleAPIScript {
        var script = feedPreviewScript(.everyone)
        var portfolio = Components.Schemas.MyPortfolio.sampleEmpty
        portfolio.cashMicros = 500_000_000
        portfolio.netWorthMicros = 500_000_000
        script.portfolio = portfolio
        script.balance = balance(micros: 500_000_000)
        script.homeCabals = []
        script.pnlHistory = .sampleEmpty
        return script
    }

    private static let acCapital = Components.Schemas.CabalRef.sampleACCapital
    private static let techTitans = Components.Schemas.CabalRef.sampleTechTitans
    private static let degens = Components.Schemas.CabalRef(
        id: "0f1d6a52-0001-7000-8000-000000000011", name: "Degens", pictureUrl: nil)
    private static let quietMoney = Components.Schemas.CabalRef(
        id: "0f1d6a52-0001-7000-8000-000000000012", name: "Quiet Money", pictureUrl: nil)

    private static func fStake(
        _ cabal: Components.Schemas.CabalRef, value: Int64, returnBps: Int64?, ownership: Int32, members: Int32
    ) -> Components.Schemas.PortfolioCabal {
        let net: Int64 = returnBps.map { value * 10_000 / (10_000 + $0) } ?? value
        return Components.Schemas.PortfolioCabal(
            cabal: cabal, valueMicros: value, shareUnits: value, netContributedMicros: net, pnlMicros: value - net,
            returnBps: value == 0 ? nil : returnBps, sliceBps: 0, ownershipBps: ownership, role: .member, canVote: true,
            potValueMicros: value * 10_000 / max(Int64(ownership), 1), memberCount: members, openVoteCount: 1)
    }

    private static var quantKidsInvite: Components.Schemas.CabalInvite {
        Components.Schemas.CabalInvite(
            requestId: "0f1d6a52-0001-7000-8000-000000000021",
            cabal: .init(
                id: "0f1d6a52-0001-7000-8000-000000000013", name: "Quant Kids", pictureUrl: nil, memberCount: 5),
            invitedBy: .init(
                userId: "0f1d6a52-0001-7000-8000-000000000022", handle: "sam", displayName: "Sam", photoUrl: nil),
            expiresAt: .now.addingTimeInterval(7 * 86_400))
    }

    private static func applyF(_ script: inout SampleAPIScript, unfunded: Bool) {
        var portfolio = Components.Schemas.MyPortfolio.sampleF
        portfolio.cabals =
            unfunded
            ? [
                fStake(acCapital, value: 7_000_000_000, returnBps: 470, ownership: 3_500, members: 8),
                fStake(quietMoney, value: 0, returnBps: nil, ownership: 0, members: 4),
            ]
            : [
                fStake(acCapital, value: 7_000_000_000, returnBps: 470, ownership: 3_500, members: 8),
                fStake(techTitans, value: 2_710_000_000, returnBps: -170, ownership: 1_200, members: 6),
                fStake(degens, value: 1_872_000_000, returnBps: 1_120, ownership: 800, members: 6),
            ]
        script.portfolio = portfolio
        script.invites = unfunded ? [] : [quantKidsInvite]
        script.callBoard = .peopleMonth
        script.catchUpCount = 6
        script.pendingVotes = script.pendingVotes.map { vote in
            var vote = vote
            vote.cabalId = acCapital.id
            vote.cabalName = acCapital.name
            return vote
        }
        script.proposals = script.proposals.mapValues { detail in
            var detail = detail
            detail.cabalId = acCapital.id
            detail.cabalName = acCapital.name
            return detail
        }
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
        switch scenario {
        case .prePrompt: { AnyView(PushPrePromptSheet(prompt: SamplePushPrePrompt.make())) }
        case .feedPreviewEveryone:
            { AnyView(ScrollView { HomeEveryonePreviewSlot.body(for: ()) }.monacoCanvas()) }
        default: nil
        }
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
