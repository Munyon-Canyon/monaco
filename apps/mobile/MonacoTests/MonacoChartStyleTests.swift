import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

enum CurveFixture {
    static let day: TimeInterval = 86_400

    static func curve(days: Int = 30, step: Int64 = -100_000_000) -> ValueCurve {
        let end = Date.now
        return ValueCurve(
            samples: (0..<days).map { index in
                ValueCurve.Sample(
                    at: end.addingTimeInterval(-Double(days - 1 - index) * day),
                    value: 1_000_000_000 + Int64(index) * step,
                    pnl: Int64(index) * step)
            })
    }
}

struct CurveChartHost: View {
    var height: CGFloat = CurveScrubChart.defaultHeight
    @State private var selection: Int?

    var body: some View {
        CurveScrubChart(
            curve: CurveFixture.curve(), plot: .pnl, range: .oneMonth, height: height, selection: $selection,
            identifier: "curve-host"
        )
    }
}

@MainActor
struct MonacoChartStyleTests {
    private static let utc = TimeZone.gmt
    private static let english = Locale(identifier: "en_US")

    private static func height(of view: some View, width: CGFloat = 390, size: DynamicTypeSize = .large) -> CGFloat {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: width, height: CGFloat.greatestFiniteMagnitude)).height
    }

    private static func dailyBars(_ count: Int) -> [PlottedBar] {
        let end = Date.now
        return (0..<count).map { index in
            let value = Double(index)
            return PlottedBar(
                date: end.addingTimeInterval(-Double(count - 1 - index) * CurveFixture.day),
                open: value, high: value, low: value, close: value)
        }
    }

    private static func dayLabel(_ date: Date) -> String {
        date.formatted(Date.FormatStyle(locale: english, timeZone: utc).month(.abbreviated).day())
    }

    private static func labels(_ bars: [PlottedBar], _ window: ChartWindow) -> (first: String, last: String)? {
        MonacoChartDateRow.labels(
            bars: bars, window: window, intervalSeconds: 86_400, locale: english, timeZone: utc)
    }

    @Test func theLineKeepsTheDesignsStrokeAndEndDot() {
        #expect(MonacoLineStyle.stroke.lineWidth == 1.7)
        #expect(MonacoLineStyle.endDotRadius == 3.5)
    }

    @Test func theDateRowNamesTheFirstAndLastBarOnShow() {
        let bars = Self.dailyBars(30)
        let window = ChartWindow.initial(bars: bars, intervalSeconds: 86_400, visibleBars: bars.count)

        let labels = Self.labels(bars, window)

        #expect(labels?.first == Self.dayLabel(bars[0].date))
        #expect(labels?.last == Self.dayLabel(bars[29].date))
    }

    @Test func theDateRowFollowsThePannedWindow() {
        let bars = Self.dailyBars(30)
        let panned = ChartWindow(
            scrollDate: bars[10].date.addingTimeInterval(-3_600), visibleSeconds: 12 * CurveFixture.day)

        let labels = Self.labels(bars, panned)

        #expect(labels?.first == Self.dayLabel(bars[10].date))
        #expect(labels?.last == Self.dayLabel(bars[21].date))
    }

    @Test func aDayLongWindowNamesTheDayWhenBothEndsReadTheSameClockTime() {
        let end = Date.now
        let bars = (0...24).map { hour in
            PlottedBar(
                date: end.addingTimeInterval(-Double(24 - hour) * 3_600), open: 1, high: 1, low: 1, close: 1)
        }
        let window = ChartWindow.initial(bars: bars, intervalSeconds: 3_600, visibleBars: bars.count)

        let labels = MonacoChartDateRow.labels(
            bars: bars, window: window, intervalSeconds: 3_600, locale: Self.english, timeZone: Self.utc)

        #expect(labels?.first != labels?.last, "labels: \(String(describing: labels))")
    }

    @Test func theDateRowIsAbsentWithoutBars() {
        let window = ChartWindow.initial(bars: [], intervalSeconds: 86_400)

        #expect(Self.labels([], window) == nil)
    }

    @Test func theChartDefaultsToTheFullHeightWithASmallVariant() {
        #expect(CurveScrubChart.defaultHeight == 170)
        #expect(CurveScrubChart.smallHeight == 148)
    }

    @Test func aLongDatePairWrapsInsteadOfTruncatingAtTheLargestTextSize() {
        let long = "Thursday, September 18, 2026 at 3:00 PM"
        let wrapped = Self.height(of: MonacoChartDateRow(first: long, last: long), size: .accessibility5)
        let single = Self.height(of: MonacoChartDateRow(first: "Sep 18", last: "Oct 18"), size: .accessibility5)

        #expect(wrapped > single)
    }

    @Test func aQuietChartIsOneHundredFourTallAndGrowsWithItsText() {
        let message = "No investment history"
        let normal = Self.height(of: QuietChart(message: message))
        let large = Self.height(
            of: QuietChart(message: message + " yet, so there is no curve to draw for this range"),
            size: .accessibility5)

        #expect(normal == 104)
        #expect(large > normal)
    }

    @Test func theTooltipShowsTheSignedAmountAndFallsBackToThePercent() {
        let curve = CurveFixture.curve()
        let share = ValueCurve(
            samples: [
                ValueCurve.Sample(at: .now.addingTimeInterval(-CurveFixture.day), value: 0, pnl: 0, nav: 1_000_000),
                ValueCurve.Sample(at: .now, value: 0, pnl: 0, nav: 1_100_000),
            ])

        #expect(CurveScrubChart.tooltipAmount(curve.change(.pnl, to: 3)) == "−$300.00")
        #expect(CurveScrubChart.tooltipAmount(share.change(.nav, to: 1))?.hasSuffix("%") == true)
        #expect(CurveScrubChart.tooltipAmount(nil) == nil)
    }
}

@MainActor
enum RenderedView {
    static func labels(
        _ view: some View, size: DynamicTypeSize = .large, until ready: ([String]) -> Bool = { !$0.isEmpty }
    ) async throws -> [String] {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let scene = try #require(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
        window.makeKeyAndVisible()
        defer { TestWindow.close(window) }
        await ProposalTestWindow.until { ready(AccessibilityTree.labels(in: window)) }
        return AccessibilityTree.labels(in: window)
    }
}
