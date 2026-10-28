#if DEBUG
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

    static let launchArgument = "-MonacoShareSample"

    static func matching(_ arguments: [String]) -> ShareSampleCase? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ShareSampleCase(rawValue: arguments[flag + 1])
    }

    var cardSize: CGSize {
        self == .square ? CGSize(width: 360, height: 360) : CGSize(width: 360, height: 450)
    }

    var spec: ShareCardSpec? {
        switch self {
        case .post, .square: ShareSampleData.spec()
        case .pnlOff: ShareSampleData.spec(eyes: ShareEyes(pnlDollars: false))
        case .buySellOff: ShareSampleData.spec(eyes: ShareEyes(buySell: false))
        case .truncated: ShareSampleData.spec(truncated: true)
        case .loading, .failed: nil
        }
    }
}

enum ShareSampleData {
    static let now = date(2026, 10, 22)

    static func spec(eyes: ShareEyes = ShareEyes(), truncated: Bool = false) -> ShareCardSpec {
        SharePositionBuilder.build(
            position: position, trades: trades, chart: chart, code: "andres", eyes: eyes, truncated: truncated,
            now: now, timeZone: utc)
    }

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
        MonacoTheme.canvas
            .ignoresSafeArea()
            .overlay { content }
            .environment(\.colorScheme, .dark)
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
            if let spec = sample.spec {
                FShareCard(spec: spec, size: sample.cardSize)
            }
        }
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
