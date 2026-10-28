import XCTest

@testable import MonacoCore

final class ChartIntervalTests: XCTestCase {
    func testSeconds_matchTheWireNames() {
        XCTAssertEqual(ChartInterval.allCases.map(\.rawValue), ["1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w"])
        XCTAssertEqual(ChartInterval.allCases.map(\.seconds), [60, 300, 900, 1_800, 3_600, 14_400, 86_400, 604_800])
        XCTAssertEqual(ChartInterval.h4.label, "4h")
    }

    func testInitSeconds_roundTripsAndRejectsUnknownWidths() {
        for interval in ChartInterval.allCases {
            XCTAssertEqual(ChartInterval(seconds: interval.seconds), interval)
        }
        XCTAssertNil(ChartInterval(seconds: 120))
    }

    func testDefault_isTheBucketTheServerUsedBeforeIntervalsExisted() {
        XCTAssertEqual(ChartInterval.default(for: .oneHour), .m1)
        XCTAssertEqual(ChartInterval.default(for: .oneDay), .m5)
        XCTAssertEqual(ChartInterval.default(for: .oneWeek), .h1)
        XCTAssertEqual(ChartInterval.default(for: .oneMonth), .h1)
        XCTAssertEqual(ChartInterval.default(for: .threeMonths), .h1)
        XCTAssertEqual(ChartInterval.default(for: .oneYear), .d1)
        XCTAssertEqual(ChartInterval.default(for: .all), .d1)
    }

    func testAllowed_followsTheServerTable() {
        XCTAssertEqual(ChartInterval.allowed(for: .oneHour), [.m1, .m5])
        XCTAssertEqual(ChartInterval.allowed(for: .oneDay), [.m5, .m15, .m30, .h1, .h4, .d1])
        XCTAssertEqual(ChartInterval.allowed(for: .oneWeek), [.m5, .m15, .m30, .h1, .h4, .d1])
        XCTAssertEqual(ChartInterval.allowed(for: .oneMonth), [.m15, .m30, .h1, .h4, .d1])
        XCTAssertEqual(ChartInterval.allowed(for: .threeMonths), [.m30, .h1, .h4, .d1, .w1])
        XCTAssertEqual(ChartInterval.allowed(for: .oneYear), [.h4, .d1, .w1])
        XCTAssertEqual(ChartInterval.allowed(for: .all), [.h4, .d1, .w1])
    }

    func testEveryRangeAllowsItsOwnDefault() {
        for range in AssetChartRange.allCases {
            XCTAssertTrue(ChartInterval.allowed(for: range).contains(.default(for: range)), "\(range)")
        }
    }

    func testPreferred_isAllowedAndKeepsEveryPresetWindowUnderTheDefaultCap() {
        for range in AssetChartRange.allCases {
            XCTAssertTrue(ChartInterval.allowed(for: range).contains(.preferred(for: range)), "\(range)")
        }
        XCTAssertEqual(ChartInterval.preferred(for: .oneDay), .m15)
        XCTAssertEqual(ChartInterval.preferred(for: .oneMonth), .h4)
        XCTAssertEqual(ChartInterval.preferred(for: .threeMonths), .d1)
        XCTAssertEqual(ChartInterval.preferred(for: .sixMonths), .w1)
        XCTAssertEqual(ChartInterval.preferred(for: .oneYear), .w1)
        XCTAssertEqual(ChartInterval.preferred(for: .all), .w1)
    }
}
