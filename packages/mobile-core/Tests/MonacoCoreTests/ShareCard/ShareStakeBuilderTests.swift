import Foundation
import XCTest

@testable import MonacoCore

final class ShareStakeBuilderTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt

    private static func date(_ year: Int, _ month: Int, _ day: Int) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar.date(from: DateComponents(year: year, month: month, day: day, hour: 15)) ?? .distantPast
    }

    private var crest: ShareCrest { ShareCrest(cabalID: "cabal-1", name: "AC Capital", pictureURL: nil) }

    private func stake(funded: Date? = date(2026, 8, 4), net: Int64 = 6_687_600_000, pnl: Int64 = 312_400_000)
        -> ShareStake
    {
        ShareStake(
            crest: crest, name: "Andres", photoURL: nil, ownershipBps: 3_500, valueMicros: 7_000_000_000,
            netContributedMicros: net, pnlMicros: pnl, firstFundedAt: funded)
    }

    private var chart: [ShareChartPoint] {
        [
            ShareChartPoint(time: Self.date(2026, 7, 1), priceMicros: 0),
            ShareChartPoint(time: Self.date(2026, 8, 4), priceMicros: 3_000_000_000),
            ShareChartPoint(time: Self.date(2026, 9, 1), priceMicros: 3_100_000_000),
            ShareChartPoint(time: Self.date(2026, 9, 2), priceMicros: 6_700_000_000),
            ShareChartPoint(time: Self.date(2026, 10, 22), priceMicros: 7_000_000_000),
        ]
    }

    private func build(
        stake: ShareStake? = nil, eyes: ShareEyes = ShareEyes(pnlDollars: true, buySell: false, code: false)
    ) -> ShareCardSpec {
        ShareStakeBuilder.build(
            stake: stake ?? self.stake(), chart: chart, code: "andres", eyes: eyes, timeZone: utc)
    }

    func testTheFrameBuildsFromTheFixtures() {
        let spec = build()

        XCTAssertEqual(spec.header.title, "Andres")
        XCTAssertEqual(spec.header.subtitle, "35% of AC Capital")
        XCTAssertEqual(spec.header.dateText, "Since Aug 2026")
        XCTAssertEqual(spec.header.mark, .photo(url: nil, name: "Andres", crest: crest))
        XCTAssertEqual(spec.figure.main, "+$312.40")
        XCTAssertEqual(spec.figure.secondary, "+4.7%")
        XCTAssertEqual(spec.figure.tone, .gain)
        XCTAssertEqual(spec.stats.map(\.label), ["Share", "Funded", "Now"])
        XCTAssertEqual(spec.stats.map(\.value), ["35%", "$6,688", "$7,000"])
        XCTAssertEqual(spec.replayTitle, "Replay your share")
        XCTAssertNil(spec.code)
        XCTAssertTrue(spec.chart.markers.isEmpty)
        XCTAssertEqual(spec.spoken, "Andres, 35% of AC Capital, up 312 dollars 40, 4.7 percent")
    }

    func testPnlDollarsOffMakesThePercentTheFigure() {
        let spec = build(eyes: ShareEyes(pnlDollars: false, buySell: false, code: false))

        XCTAssertEqual(spec.figure.main, "+4.7%")
        XCTAssertNil(spec.figure.secondary)
        XCTAssertEqual(spec.spoken, "Andres, 35% of AC Capital, up 4.7 percent")
    }

    func testCodeOnAddsTheFooterCode() {
        XCTAssertEqual(build(eyes: ShareEyes(pnlDollars: true, buySell: false, code: true)).code, "ANDRES")
    }

    func testBuySellAddsNoMarkersToAStake() {
        XCTAssertTrue(build(eyes: ShareEyes()).chart.markers.isEmpty)
    }

    func testTheChartIsTheStakesEquityFromTheFirstFundOn() {
        let points = build().chart.points

        XCTAssertEqual(points.map(\.time), chart.dropFirst().map(\.time))
        XCTAssertEqual(points.map(\.priceMicros), [3_000_000_000, 3_100_000_000, 6_700_000_000, 7_000_000_000])
    }

    func testAStakeFundedTwiceShowsTheStepAtTheSecondFund() {
        let values = build().chart.points.map(\.priceMicros)

        XCTAssertEqual(values[2] - values[1], 3_600_000_000)
    }

    func testAChartWithTooFewPointsSinceTheFundKeepsTheWholeCurve() {
        let spec = ShareStakeBuilder.build(
            stake: stake(funded: Self.date(2026, 10, 22)), chart: chart, code: nil, eyes: ShareEyes(), timeZone: utc)

        XCTAssertEqual(spec.chart.points.count, chart.count)
    }

    func testNoFirstFundLeavesTheDateEmptyAndKeepsTheCurve() {
        let spec = build(stake: stake(funded: nil))

        XCTAssertEqual(spec.header.dateText, "")
        XCTAssertEqual(spec.chart.points.count, chart.count)
    }

    func testNothingContributedHasNoPercent() {
        let spec = build(stake: stake(net: 0, pnl: 0))

        XCTAssertEqual(spec.figure.secondary, "\u{2014}")
    }

    func testALossIsTheLossTone() {
        let spec = build(stake: stake(pnl: -312_400_000))

        XCTAssertEqual(spec.figure.tone, .loss)
        XCTAssertTrue(spec.spoken.contains("down 312 dollars 40"))
    }

    func testAFractionalOwnershipKeepsOneDigit() {
        XCTAssertEqual(ShareStakeBuilder.ownershipText(3_550), "35.5%")
    }
}
