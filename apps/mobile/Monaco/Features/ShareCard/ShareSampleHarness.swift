#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ShareSampleCase: String, CaseIterable {
    case post
    case square
    case pnlOff
    case buySellOff
    case truncated
    case loading
    case failed
    case screenPost
    case screenSquare
    case screenStory
    case screenSticker
    case screenPnlOff
    case screenBuySellOff
    case screenCodeOff
    case screenReplay
    case screenLoading
    case screenFailed
    case passed
    case passedPnlOn
    case passedNoPassedPrice
    case passedPending
    case passedFailed
    case passedNoLivePrice
    case vote
    case screenPassed
    case screenVote
    case stake
    case portfolio
    case stickerOff
    case stickerOn
    case stickerPassed
    case screenStake
    case screenPortfolio

    static let launchArgument = "-MonacoShareSample"

    static func matching(_ arguments: [String]) -> ShareSampleCase? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ShareSampleCase(rawValue: arguments[flag + 1])
    }

    var cardSize: CGSize {
        switch self {
        case .square, .passed, .passedPnlOn, .passedNoPassedPrice, .passedPending, .passedFailed, .passedNoLivePrice,
            .vote:
            CGSize(width: 360, height: 360)
        default:
            CGSize(width: 360, height: 450)
        }
    }

    var spec: ShareCardSpec? {
        switch self {
        case .passed, .screenPassed, .stickerPassed: ShareSampleData.passed()
        case .passedPnlOn: ShareSampleData.passed(eyes: ShareEyes(pnlDollars: true))
        case .passedNoPassedPrice: ShareSampleData.passed(passedPrice: nil)
        case .passedPending: ShareSampleData.passed(outcome: .pending)
        case .passedFailed: ShareSampleData.passed(outcome: .failed)
        case .passedNoLivePrice: ShareSampleData.passed(outcome: .noLivePrice)
        case .vote, .screenVote: ShareSampleData.vote()
        case .stake, .screenStake: ShareSampleData.stake()
        case .portfolio, .screenPortfolio: ShareSampleData.portfolio()
        case .stickerOff, .stickerOn: ShareSampleData.spec(eyes: stickerEyes)
        case .post, .square: ShareSampleData.spec()
        case .pnlOff: ShareSampleData.spec(eyes: ShareEyes(pnlDollars: false))
        case .buySellOff: ShareSampleData.spec(eyes: ShareEyes(buySell: false))
        case .truncated: ShareSampleData.spec(truncated: true)
        case .loading, .failed: nil
        default: ShareSampleData.spec(eyes: screenEyes)
        }
    }

    var isSticker: Bool { [.stickerOff, .stickerOn, .stickerPassed].contains(self) }

    var stickerEyes: ShareEyes { self == .stickerOn ? ShareEyes() : ShareScreenModel.stickerEyes }

    var isScreen: Bool { rawValue.hasPrefix("screen") || self == .stickerOn }

    var screenFormat: ShareFormat {
        switch self {
        case .screenPassed, .screenVote: .square
        case .screenSquare: .square
        case .screenStory: .story
        case .screenSticker, .stickerOn: .sticker
        default: .post
        }
    }

    var screenEyes: ShareEyes {
        switch self {
        case .screenPassed: ShareEyes(pnlDollars: false)
        case .screenVote: ShareEyes(pnlDollars: false, buySell: false)
        case .screenPnlOff: ShareEyes(pnlDollars: false)
        case .screenBuySellOff: ShareEyes(buySell: false)
        case .screenCodeOff: ShareEyes(code: false)
        case .screenStake: ShareEyes(pnlDollars: true, buySell: false, code: false)
        case .screenSticker: ShareScreenModel.stickerEyes
        case .screenPortfolio: ShareEyes(pnlDollars: true, buySell: false)
        default: ShareEyes()
        }
    }

    var replayProgress: Double? { self == .screenReplay ? 0.5 : nil }

    var screenTitle: String {
        switch self {
        case .screenVote: ShareScreenModel.voteTitle
        case .screenSticker, .stickerOn: ShareScreenModel.stickerTitle
        default: ShareScreenModel.positionTitle
        }
    }

    var unusedEyes: Set<ShareEye> { self == .screenVote ? [.pnl, .trades] : [] }
}

enum ShareSampleData {
    static let now = date(2026, 10, 22)

    static func spec(eyes: ShareEyes = ShareEyes(), truncated: Bool = false) -> ShareCardSpec {
        SharePositionBuilder.build(
            position: position, trades: trades, chart: chart, code: "andres", eyes: eyes, truncated: truncated,
            now: now, timeZone: utc)
    }

    enum PassedOutcome {
        case filled
        case noLivePrice
        case pending
        case failed
    }

    static func passed(
        eyes: ShareEyes = ShareEyes(pnlDollars: false), passedPrice: Int64? = 709_800_000,
        outcome: PassedOutcome = .filled
    ) -> ShareCardSpec {
        var detail = Components.Schemas.ProposalDetail.filledMETA(now: now)
        detail.passedPriceMicros = passedPrice
        detail.createdAt = now.addingTimeInterval(-39_600)
        detail.stats.closesAt = now.addingTimeInterval(-27_600)
        detail.stats.closedAt = now.addingTimeInterval(-27_600)
        detail.swap?.filledAt = now.addingTimeInterval(-21_600)
        detail.cabalName = "Degens"
        detail.symbol = "META"
        detail.tally = .init(yes: 4, no: 1, voters: 6, needed: 4)
        detail.proposer = .init(id: detail.proposerId, displayName: "Christelle", photoUrl: nil)
        switch outcome {
        case .filled:
            break
        case .noLivePrice:
            detail.stats.priceMicros = nil
            detail.stats.sinceBps = nil
        case .pending:
            detail.swap = nil
            detail.status = .passed
            detail.stats.sinceBps = nil
        case .failed:
            detail.status = .failed
            detail.swap?.status = .failed
            detail.swap?.priceMicros = nil
            detail.swap?.filledAt = nil
            detail.stats.sinceBps = nil
        }
        return SharePassedBuilder.build(
            proposal: detail, crest: ShareCrest(cabalID: "degens", name: "Degens", pictureURL: nil),
            proposerPhoto: nil, chart: metaChart, code: "andres", eyes: eyes, timeZone: utc)
    }

    static func vote(eyes: ShareEyes = ShareEyes(pnlDollars: false, buySell: false)) -> ShareCardSpec {
        var detail = Components.Schemas.ProposalDetail.openNVDA(now: now)
        detail.proposer = .init(id: detail.proposerId, displayName: "Mia", photoUrl: nil)
        detail.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        return ShareVoteBuilder.build(
            proposal: detail, crest: ShareCrest(cabalID: "ac-capital", name: "AC Capital", pictureURL: nil),
            chart: nvdaDay, code: "andres", eyes: eyes, now: now, timeZone: utc)
    }

    static func stake(eyes: ShareEyes = ShareEyes(pnlDollars: true, buySell: false, code: false)) -> ShareCardSpec {
        let stake = ShareStake(
            crest: ShareCrest(cabalID: "ac-capital", name: "AC Capital", pictureURL: nil), name: "Andres",
            photoURL: nil, ownershipBps: 3_500, valueMicros: 7_000_000_000, netContributedMicros: 6_687_600_000,
            pnlMicros: 312_400_000, firstFundedAt: date(2026, 8, 4))
        let start = date(2026, 8, 4)
        let values: [Int64] = [3_000, 3_080, 3_020, 3_150, 6_700, 6_620, 6_850, 6_790, 7_000]
        let points = values.enumerated().map {
            ShareChartPoint(
                time: start.addingTimeInterval(TimeInterval($0.offset) * 8 * 86_400),
                priceMicros: $0.element * 1_000_000)
        }
        return ShareStakeBuilder.build(stake: stake, chart: points, code: "andres", eyes: eyes, timeZone: utc)
    }

    static func portfolio(eyes: ShareEyes = ShareEyes(pnlDollars: true, buySell: false)) -> ShareCardSpec {
        let portfolio = SharePortfolio(
            name: "Andres", photoURL: nil, cabalCount: 3, totalValueMicros: 12_082_000_000, pnl1mMicros: 41_250_000,
            return1mBps: 820, rightCalls: 7, resolvedCalls: 9)
        let start = now.addingTimeInterval(-30 * 86_400)
        let values: [Int64] = [900, 905, 898, 920, 915, 930, 925, 941]
        let points = values.enumerated().map {
            ShareChartPoint(
                time: start.addingTimeInterval(TimeInterval($0.offset) * 4 * 86_400),
                priceMicros: $0.element * 1_000_000)
        }
        return SharePortfolioBuilder.build(
            portfolio: portfolio, history: points, code: "andres", eyes: eyes, now: now, timeZone: utc)
    }

    private static let metaChart: [ShareChartPoint] = {
        let start = now.addingTimeInterval(-39_600)
        let cents: [Int64] = [
            70_200, 70_450, 70_700, 70_920, 71_100, 71_650, 71_800, 72_000, 72_150, 72_300, 72_250, 72_440,
        ]
        return cents.enumerated().map {
            let time = start.addingTimeInterval(TimeInterval($0.offset) * 3_600)
            return ShareChartPoint(time: time, priceMicros: $0.element * 10_000)
        }
    }()

    private static let nvdaDay: [ShareChartPoint] = {
        let start = now.addingTimeInterval(-86_400)
        let prices: [Int64] = [214, 216, 215, 218, 217, 219, 218, 221, 220, 222, 221, 220]
        return prices.enumerated().map {
            let time = start.addingTimeInterval(TimeInterval($0.offset) * 7_200)
            return ShareChartPoint(time: time, priceMicros: $0.element * 1_000_000)
        }
    }()

    private static let utc = TimeZone(identifier: "UTC") ?? .gmt

    private static func date(_ year: Int, _ month: Int, _ day: Int) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = utc
        return calendar.date(from: DateComponents(year: year, month: month, day: day, hour: 15)) ?? .distantPast
    }

    private static let position = SharePosition(
        crest: ShareCrest(cabalID: "ac-capital", name: "AC Capital", pictureURL: nil), symbol: "NVDA", logoURL: nil,
        unitsText: "38.0000", valueMicros: 8_360_000_000, costBasisMicros: 7_332_100_000, pnlMicros: 1_027_900_000,
        avgEntryMicros: 192_950_000, priceMicros: 220_000_000, firstBoughtAt: date(2026, 6, 3))

    private static let trades = [
        ShareTrade(isBuy: true, time: date(2026, 6, 3), priceMicros: 181_200_000),
        ShareTrade(isBuy: true, time: date(2026, 7, 14), priceMicros: 188_400_000),
        ShareTrade(isBuy: false, time: date(2026, 8, 20), priceMicros: 214_100_000),
        ShareTrade(isBuy: true, time: date(2026, 9, 30), priceMicros: 196_750_000),
    ]

    private static let chart: [ShareChartPoint] = {
        let start = date(2026, 6, 3)
        let days: [(Int64, Int64)] = [
            (0, 18_120), (10, 17_800), (20, 18_400), (30, 18_300), (41, 18_840), (50, 19_500), (60, 20_400),
            (70, 21_100), (78, 21_410), (85, 20_800), (95, 20_100), (105, 19_700), (119, 19_675), (125, 20_500),
            (135, 21_400), (141, 22_000),
        ]
        return days.map {
            ShareChartPoint(time: start.addingTimeInterval(TimeInterval($0.0 * 86_400)), priceMicros: $0.1 * 10_000)
        }
    }()
}

struct ShareSampleHarness: View {
    let sample: ShareSampleCase

    var body: some View {
        if sample.isScreen {
            ShareScreenSample(sample: sample)
        } else {
            MonacoTheme.canvas
                .ignoresSafeArea()
                .overlay { content }
                .environment(\.colorScheme, .dark)
        }
    }

    @ViewBuilder
    private var content: some View {
        switch sample {
        case .loading:
            ProgressView().accessibilityIdentifier("share-loading")
        case .failed:
            Text("Couldn't load the card")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("share-failed")
        default:
            if let spec = sample.spec, sample.isSticker {
                let size = CGSize(width: 360, height: FShareSticker.designHeight)
                FShareSticker(spec: ShareStickerSpec(from: spec, eyes: sample.stickerEyes), size: size)
                    .padding(8)
                    .background(ShareCheckerboard())
            } else if let spec = sample.spec {
                FShareCard(spec: spec, size: sample.cardSize)
            }
        }
    }
}

private struct ShareScreenSample: View {
    let sample: ShareSampleCase
    @State private var eyes: ShareEyes
    @State private var format: ShareFormat
    @State private var path = [0]

    init(sample: ShareSampleCase) {
        self.sample = sample
        _eyes = State(initialValue: sample.screenEyes)
        _format = State(initialValue: sample.screenFormat)
    }

    private var state: LoadState<ShareCardSpec> {
        switch sample {
        case .screenLoading: .loading
        case .screenFailed: .failed(.decoding("no card"))
        case .screenPassed: .loaded(ShareSampleData.passed(eyes: eyes))
        case .screenVote: .loaded(ShareSampleData.vote(eyes: eyes))
        case .screenStake: .loaded(ShareSampleData.stake(eyes: eyes))
        case .screenPortfolio: .loaded(ShareSampleData.portfolio(eyes: eyes))
        default: .loaded(ShareSampleData.spec(eyes: eyes))
        }
    }

    var body: some View {
        NavigationStack(path: $path) {
            MonacoTheme.canvas
                .navigationDestination(for: Int.self) { _ in
                    ShareScreenContent(
                        state: state, eyes: $eyes, format: $format, frozenProgress: sample.replayProgress,
                        unusedEyes: sample.unusedEyes,
                        share: { _, _ in }, copyImage: { _ in }, retry: {}
                    )
                    .monacoCanvas()
                    .navigationTitle(format == .sticker ? ShareScreenModel.stickerTitle : sample.screenTitle)
                    .navigationBarTitleDisplayMode(.inline)
                }
        }
        .environment(\.colorScheme, .dark)
    }
}

final class ShareSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let sample = ShareSampleCase.matching(arguments) else { return nil }
        return AnyView(ShareSampleHarness(sample: sample))
    }
}
#endif
