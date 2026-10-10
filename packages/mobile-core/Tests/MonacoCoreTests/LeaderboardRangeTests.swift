import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class LeaderboardRangeTests: XCTestCase {
    func testEveryGeneratedRangeMapsToOneCaseAndBack() {
        let generated = Components.Parameters.LeaderboardRange.allCases
        XCTAssertEqual(generated.count, LeaderboardRange.allCases.count)
        for value in generated {
            let range = LeaderboardRange(value)
            XCTAssertEqual(range.rawValue, value.rawValue)
            XCTAssertEqual(range.generated, value)
            XCTAssertEqual(range.valueHistory.rawValue, value.rawValue)
        }
    }

    func testLabelsAndPhrases() {
        XCTAssertEqual(LeaderboardRange.allCases.map(\.label), ["1H", "1D", "1W", "1M", "All"])
        XCTAssertEqual(LeaderboardRange.oneWeek.windowPhrase, "the past week")
        XCTAssertEqual(LeaderboardRange.all.windowPhrase, "all time")
    }

    func testEveryRangeDatesItsScrubLikeTheStockChart() {
        XCTAssertEqual(
            LeaderboardRange.allCases.map(\.scrubLabelRange), [.oneDay, .oneDay, .oneWeek, .oneMonth, .all])

        let tuesday = Date(timeIntervalSince1970: 1_790_085_600)
        func caption(_ range: LeaderboardRange) -> String {
            ChartScrubLabel.caption(
                for: tuesday, range: range.scrubLabelRange, locale: Locale(identifier: "en_US"),
                timeZone: .gmt)
        }

        let month = caption(.oneMonth)
        XCTAssertTrue(month.contains("Sep"), month)
        XCTAssertFalse(month.contains(":"), month)
        XCTAssertTrue(caption(.all).contains("2026"), caption(.all))
    }
}
