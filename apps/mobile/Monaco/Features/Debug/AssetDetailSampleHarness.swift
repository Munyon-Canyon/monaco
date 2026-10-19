#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum AssetDetailSampleScenario: String, CaseIterable {
    case open
    case loss
    case candles
    case candlesLong
    case candlesSparse
    case noHolders
    case noProposals
    case preMarket
    case afterHours
    case closed
    case preIpo
    case untradable
    case paused
    case stale
    case emptyChart
    case chartFailed
    case loading
    case failed

    static let launchArgument = "-MonacoAssetDetailSample"
    private static let staleMinutes: TimeInterval = 14

    static func matching(_ arguments: [String]) -> AssetDetailSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return AssetDetailSampleScenario(rawValue: arguments[flag + 1])
    }

    var showsSampleProposal: Bool {
        switch self {
        case .open, .loss, .candles, .candlesLong, .candlesSparse, .stale: true
        default: false
        }
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        switch self {
        case .open, .candles, .noProposals: break
        case .loss: script.makeLoss()
        case .candlesLong: script.candleCount = 2_200
        case .candlesSparse: script.sparseCandles = true
        case .noHolders: script.cabalCount = 0
        case .preMarket: script.asset.session = .preMarket
        case .afterHours: script.asset.session = .afterHours
        case .closed:
            script.asset.session = .init(
                state: .closed, continuous: false, holiday: "Thanksgiving Day", earlyClose: false)
        case .preIpo: script.asset = .spaceX
        case .untradable: script.asset.tradable = false
        case .paused: script.asset = .paused
        case .stale: break
        case .emptyChart: script.chart = .empty
        case .chartFailed: script.mode = .chartUnavailable
        case .loading: script.mode = .hang
        case .failed: script.mode = .assetsUnavailable
        }
        let age = self == .stale ? Self.staleMinutes * 60 : 0
        script.asset.priceAsOf = Date().addingTimeInterval(-age)
        if showsSampleProposal { script.proposal = .sample(canVote: true) }
        return script
    }
}

extension SampleAPIScript {
    fileprivate mutating func makeLoss() {
        asset.changeBps = -444
        var state: UInt64 = 0x9E37_79B9_7F4A_7C15
        func next(_ bound: Int64) -> Int64 {
            state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
            return Int64(state >> 33) % bound
        }
        let start: Int64 = 172_800_000
        let lastBar: Int64 = 1_790_085_300
        var close = start
        var points: [Components.Schemas.ChartPoint] = []
        for index in 0..<candleCount {
            let open = close
            close = max(open + next(1_600_000) - 823_000, 1_000_000)
            let high = max(open, close) + next(700_000)
            let low = min(open, close) - next(700_000)
            points.append(
                Components.Schemas.ChartPoint(
                    t: Date(timeIntervalSince1970: TimeInterval(lastBar - Int64(candleCount - 1 - index) * 300)),
                    openMicros: open, highMicros: high, lowMicros: low, closeMicros: close))
        }
        chart = Components.Schemas.AssetChart(
            range: ._1d, interval: ._5m, bucketSeconds: 300, points: points, empty: false,
            attribution: "Data provided by CoinGecko", nextBefore: nil, previousCloseMicros: start)
    }
}

struct AssetSampleProposalCard: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var card: Card?

    private struct Card {
        let detail: ProposalDetail
        let asset: ProposalAsset
        let members: [ProposalMember]
    }

    var body: some View {
        ZStack {
            if let card {
                ProposalCard(proposal: card.detail, asset: card.asset, members: card.members)
            }
        }
        .task { await load() }
    }

    private func load() async {
        let repository = ProposalsRepository(api: environment.api)
        guard let detail = try? await repository.detail(id: "proposal-1"),
            let asset = try? await repository.asset(symbol: detail.summary.symbol),
            let members = try? await repository.members(cabalID: detail.summary.cabalID)
        else { return }
        card = Card(detail: detail, asset: asset, members: members)
    }
}

final class AssetDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = AssetDetailSampleScenario.matching(arguments) else { return nil }
        let script = scenario.script
        SampleAPIProtocol.install(script)
        MarketsSampleCatalog.install(watched: scenario == .open ? [script.asset.symbol] : [])
        AssetProposals.sampleCard = scenario.showsSampleProposal ? AnyView(AssetSampleProposalCard()) : nil
        return AnyView(
            SampleAppFrame(auth: auth, tab: .stocks, routes: [AssetRoute(symbol: script.asset.symbol)])
        )
    }
}
#endif
