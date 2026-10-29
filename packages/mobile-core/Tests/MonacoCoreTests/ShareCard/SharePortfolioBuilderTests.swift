import Foundation
import XCTest

@testable import MonacoCore

final class SharePortfolioBuilderTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt

    private static func date(_ month: Int, _ day: Int) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar.date(from: DateComponents(year: 2026, month: month, day: day, hour: 15)) ?? .distantPast
    }

    private func portfolio(
        cabals: Int = 3, bps: Int64? = 820, right: Int? = 7, resolved: Int? = 9, pnl: Int64 = 41_250_000
    ) -> SharePortfolio {
        SharePortfolio(
            name: "Andres", photoURL: nil, cabalCount: cabals, totalValueMicros: 12_082_000_000, pnl1mMicros: pnl,
            return1mBps: bps, rightCalls: right, resolvedCalls: resolved)
    }

    private var history: [ShareChartPoint] {
        [
            ShareChartPoint(time: Self.date(9, 22), priceMicros: 900_000_000),
            ShareChartPoint(time: Self.date(10, 5), priceMicros: 910_000_000),
            ShareChartPoint(time: Self.date(10, 22), priceMicros: 950_000_000),
        ]
    }

    private func build(
        _ portfolio: SharePortfolio? = nil, history: [ShareChartPoint]? = nil, eyes: ShareEyes = ShareEyes()
    ) -> ShareCardSpec {
        SharePortfolioBuilder.build(
            portfolio: portfolio ?? self.portfolio(), history: history ?? self.history, code: "andres", eyes: eyes,
            now: Self.date(10, 22), timeZone: utc)
    }

    func testTheCardBuildsFromTheServersMonthFigures() {
        let spec = build()

        XCTAssertEqual(spec.header.title, "Andres")
        XCTAssertEqual(spec.header.subtitle, "3 cabals")
        XCTAssertEqual(spec.header.dateText, "Oct 22, 2026")
        XCTAssertEqual(spec.figure.main, "+$41.25")
        XCTAssertEqual(spec.figure.secondary, "+8.2%")
        XCTAssertEqual(spec.stats.map(\.label), ["Positions", "P&L \u{00B7} 1M", "Right calls"])
        XCTAssertEqual(spec.stats.map(\.value), ["$12,082", "+8.2%", "7 of 9"])
        XCTAssertEqual(spec.code, "ANDRES")
        XCTAssertEqual(spec.replayTitle, "Replay your month")
        XCTAssertEqual(spec.spoken, "Andres, 3 cabals, past month, up 41 dollars 25, 8.2 percent")
    }

    func testNoStandingIsADash() {
        let spec = build(portfolio(right: nil, resolved: nil))

        XCTAssertEqual(spec.stats.last?.value, "\u{2014}")
    }

    func testOneCabalIsSingular() {
        XCTAssertEqual(build(portfolio(cabals: 1)).header.subtitle, "1 cabal")
    }

    func testProfitsFromBeforeTheMonthLeaveTheFigureAtTheServers1MAndTheChartAtZero() {
        let spec = build()

        XCTAssertEqual(spec.figure.main, "+$41.25")
        XCTAssertEqual(spec.chart.points.map(\.priceMicros), [0, 10_000_000, 50_000_000])
    }

    func testAnEmptyHistoryHasNoChart() {
        XCTAssertTrue(build(history: []).chart.points.isEmpty)
    }

    func testAnUnsortedHistoryIsRebasedOnItsEarliestPoint() {
        let spec = build(history: history.reversed())

        XCTAssertEqual(spec.chart.points.map(\.priceMicros), [0, 10_000_000, 50_000_000])
    }

    func testPnlDollarsOffAndNoReturn() {
        let spec = build(portfolio(bps: nil), eyes: ShareEyes(pnlDollars: false, buySell: false, code: false))

        XCTAssertEqual(spec.figure.main, "\u{2014}")
        XCTAssertNil(spec.code)
    }

    func testALossIsTheLossTone() {
        XCTAssertEqual(build(portfolio(bps: -250, pnl: -250_000_000)).figure.tone, .loss)
    }
}
