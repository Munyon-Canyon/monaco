#if DEBUG
import MonacoCore
import SwiftUI

enum GalleryTrend: String, CaseIterable {
    case gain, loss, flat

    private static let wave: [Int64] = [0, 5, 9, 11, 10, 7, 2, -3, -7, -8, -6, -2, 2, 6, 8, 7, 4, 0, -4, -6]

    var curve: ValueCurve {
        let days = 30
        let end = Date.now
        return ValueCurve(
            samples: (0..<days).map { index in
                let step = Int64(index)
                let wiggle = Self.wave[index % Self.wave.count] * 3_000_000
                let pnl: Int64 =
                    switch self {
                    case .gain: step * 9_000_000 + wiggle
                    case .loss: -step * 7_000_000 + wiggle
                    case .flat: 0
                    }
                return ValueCurve.Sample(
                    at: end.addingTimeInterval(-TimeInterval(days - 1 - index) * 86_400), value: 1_000_000_000 + pnl,
                    pnl: pnl)
            })
    }
}

struct GalleryCurveChart: View {
    let trend: GalleryTrend
    let height: CGFloat
    private let curve: ValueCurve
    @State private var selection: Int? = 17

    init(trend: GalleryTrend, height: CGFloat) {
        self.trend = trend
        self.height = height
        curve = trend.curve
    }

    var body: some View {
        GalleryCase(caption: "\(trend.rawValue), \(Int(height))") {
            CurveScrubChart(
                curve: curve, plot: .pnl, range: .oneMonth, height: height, selection: $selection,
                identifier: "gallery-chart-\(trend.rawValue)-\(Int(height))")
        }
    }
}

struct GalleryCurveChartAtRest: View {
    let trend: GalleryTrend
    private let curve: ValueCurve
    @State private var selection: Int?

    init(trend: GalleryTrend) {
        self.trend = trend
        curve = trend.curve
    }

    var body: some View {
        GalleryCase(caption: "\(trend.rawValue), at rest") {
            CurveScrubChart(
                curve: curve, plot: .pnl, range: .oneMonth, height: CurveScrubChart.defaultHeight,
                selection: $selection, identifier: "gallery-chart-\(trend.rawValue)-rest")
        }
    }
}

struct GallerySparklineRow: View {
    var body: some View {
        GalleryCase(caption: "Sparklines, line only") {
            HStack(spacing: MonacoTheme.Space.l) {
                ForEach([PnLTone.profit, .loss, .flat], id: \.self) { tone in
                    if let series = SparklineSeries(usdcMicros: Self.points) {
                        Sparkline(series: series, tone: tone)
                    }
                }
            }
        }
    }

    private static let points: [Int64] = [10, 12, 11, 15, 14, 18, 17, 21, 19, 24].map { $0 * 1_000_000 }
}

struct GalleryChartsPage: View {
    var body: some View {
        GalleryPage(title: "Charts") {
            GalleryGroup(title: "Performance line") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    ForEach(GalleryTrend.allCases, id: \.self) { trend in
                        GalleryCurveChartAtRest(trend: trend)
                    }
                    ForEach(GalleryTrend.allCases, id: \.self) { trend in
                        GalleryCurveChart(trend: trend, height: CurveScrubChart.defaultHeight)
                    }
                    GallerySparklineRow()
                    ForEach(GalleryTrend.allCases, id: \.self) { trend in
                        GalleryCurveChart(trend: trend, height: CurveScrubChart.smallHeight)
                    }
                    GalleryCase(caption: "Quiet chart") { QuietChart(message: "No investment history") }
                }
            }
            GalleryBarSamples()
            GalleryStatSamples()
        }
    }
}

struct GalleryBarSamples: View {
    private let threeAndCash: [AllocationBar.Slice] = [
        .init(label: "NVIDIA", fraction: 0.38), .init(label: "Apple", fraction: 0.27),
        .init(label: "Tesla", fraction: 0.15), .init(label: "Cash", fraction: 0.20, isCash: true),
    ]

    private let sixHoldings: [AllocationBar.Slice] = [
        .init(label: "NVIDIA", fraction: 0.25), .init(label: "Apple", fraction: 0.20),
        .init(label: "Tesla", fraction: 0.15), .init(label: "Microsoft", fraction: 0.15),
        .init(label: "Amazon", fraction: 0.13), .init(label: "Alphabet", fraction: 0.12),
    ]

    var body: some View {
        GalleryGroup(title: "Bars") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Vote bar, open") {
                    VoteBar(yes: 3, no: 1, notVoted: 4, needed: 5, isClosed: false)
                }
                GalleryCase(caption: "Vote bar, all yes") {
                    VoteBar(yes: 8, no: 0, notVoted: 0, needed: 5, isClosed: false)
                }
                GalleryCase(caption: "Vote bar, closed") {
                    VoteBar(yes: 5, no: 2, notVoted: 1, needed: 5, isClosed: true)
                }
                GalleryCase(caption: "Allocation bar, 3 holdings and cash, with legend") {
                    AllocationBar(slices: threeAndCash)
                }
                GalleryCase(caption: "Allocation bar, 3 holdings and cash, bar only") {
                    AllocationBar(slices: threeAndCash, showsLegend: false)
                }
                GalleryCase(caption: "Allocation bar, cash only, with legend") {
                    AllocationBar(slices: [.init(label: "Cash", fraction: 1, isCash: true)])
                }
                GalleryCase(caption: "Allocation bar, 6 holdings, with legend") {
                    AllocationBar(slices: sixHoldings)
                }
            }
        }
    }
}

struct GalleryStatSamples: View {
    private let market: [StatItem] = [
        StatItem(label: "Market cap", value: "$3.21T"), StatItem(label: "Day range", value: "$402 to $418"),
        StatItem(label: "Volume", value: "24.1M"), StatItem(label: "52 week high", value: "$431.20"),
        StatItem(label: "52 week low", value: "$212.40"), StatItem(label: "Held by cabals", value: "14"),
    ]

    var body: some View {
        GalleryGroup(title: "Stats") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Stat grid, 6 cells") { StatGrid(items: market) }
                GalleryCase(caption: "Stat pair, one") {
                    StatPair(items: [StatItem(label: "Your share", value: "$311.50")])
                }
                GalleryCase(caption: "Stat pair, two") {
                    StatPair(items: [
                        StatItem(label: "Your share", value: "$311.50"), StatItem(label: "Cash", value: "$54.00"),
                    ])
                }
                GalleryCase(caption: "Money strip") {
                    MoneyStrip(netWorth: "$11,582.32", cash: "$2,044.10", depositTitle: "Deposit", onDeposit: {})
                }
            }
        }
    }
}
#endif
