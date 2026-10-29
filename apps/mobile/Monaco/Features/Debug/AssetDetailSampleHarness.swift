#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum AssetDetailSampleScenario: String, CaseIterable {
    case open
    case manyClosed
    case loss
    case statsDashes
    case candlesSparse
    case noHolders
    case noProposals
    case preMarket
    case afterHours
    case closed
    case preIpo
    case preIpoNoProfile
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

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        switch self {
        case .open, .noProposals: script.show(.nvda)
        case .manyClosed: script.show(.nvda)
        case .loss: script.makeLoss()
        case .statsDashes: script.show(.nvdaDashes)
        case .candlesSparse: script.sparseCandles = true
        case .noHolders: script.cabalCount = 0
        case .preMarket: script.asset.session = .preMarket
        case .afterHours: script.asset.session = .afterHours
        case .closed:
            script.asset.session = .init(
                state: .closed, continuous: false, holiday: "Thanksgiving Day", earlyClose: false)
        case .preIpo:
            script.show(.spaceX)
            script.showPreIpoInYourCabals()
        case .preIpoNoProfile:
            var noProfile = Components.Schemas.AssetDetail.spaceX
            noProfile.preIpo = nil
            script.show(noProfile)
            script.showPreIpoInYourCabals()
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
        if self == .open || self == .manyClosed { script.showInYourCabals(closedFirst: self == .manyClosed) }
        return script
    }
}

extension SampleAPIScript {
    fileprivate mutating func showInYourCabals(closedFirst: Bool) {
        held = .sampleAndres
        let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
        func post(_ number: Int, open: Bool, minutesAgo: Double) -> Components.Schemas.FeedItem {
            .sampleProposalPost(
                number: number, cabalID: cabalID, cabalName: "AC Capital", symbol: "NVDAx", usdcMicros: 400_000_000,
                open: open, minutesAgo: minutesAgo)
        }
        assetFeed = [post(30, open: true, minutesAgo: closedFirst ? 60 * 24 * 9 : 8)]
        if closedFirst { assetFeed += (31...35).map { post($0, open: false, minutesAgo: Double($0 - 30) * 90) } }
    }

    fileprivate mutating func showPreIpoInYourCabals() {
        held = .sampleAndres
        var post = Components.Schemas.FeedItem.sampleProposalPost(
            number: 40, cabalID: "01890a5d-ac96-774b-bcce-b302099a8062", cabalName: "Tech Titans",
            symbol: "tSpaceX", usdcMicros: 500_000_000, open: true, minutesAgo: 8)
        post.tally = .init(yes: 2, no: 0, voters: 6, needed: 4)
        post.commentCount = 3
        post.expiresAt = Date(timeIntervalSinceNow: 20 * 3600 + 600)
        assetFeed = [post]
    }

    fileprivate mutating func show(_ detail: Components.Schemas.AssetDetail) {
        asset = detail
        assets[detail.symbol] = detail
    }

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

final class AssetDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = AssetDetailSampleScenario.matching(arguments) else { return nil }
        let script = scenario.script
        SampleAPIProtocol.install(script)
        MarketsSampleCatalog.install(watched: scenario == .open ? [script.asset.symbol] : [])
        return AnyView(
            SampleAppFrame(auth: auth, tab: .stocks, routes: [AssetRoute(symbol: script.asset.symbol)])
        )
    }
}
#endif
