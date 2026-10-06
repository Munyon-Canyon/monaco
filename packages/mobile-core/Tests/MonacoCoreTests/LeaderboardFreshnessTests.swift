import Foundation
import XCTest

@testable import MonacoCore

final class LeaderboardFreshnessTests: XCTestCase {
    private let computed = Date(timeIntervalSince1970: 1_790_996_460)

    private func label(minutesAgo: Double, seconds: Double = 0) -> String {
        freshnessLabel(computedAt: computed, now: computed.addingTimeInterval(minutesAgo * 60 + seconds))
    }

    func testUnderThreeMinutesIsLive() {
        XCTAssertEqual(label(minutesAgo: 0), "Live")
        XCTAssertEqual(label(minutesAgo: 2, seconds: 59), "Live")
    }

    func testThreeMinutesOnIsMinutes() {
        XCTAssertEqual(label(minutesAgo: 3), "Updated 3 min ago")
        XCTAssertEqual(label(minutesAgo: 59, seconds: 59), "Updated 59 min ago")
    }

    func testAnHourOnIsHours() {
        XCTAssertEqual(label(minutesAgo: 60), "Updated 1 h ago")
        XCTAssertEqual(label(minutesAgo: 150), "Updated 2 h ago")
        XCTAssertEqual(label(minutesAgo: 60 * 30), "Updated 30 h ago")
    }

    func testAComputedAtInTheFutureIsLive() {
        XCTAssertEqual(label(minutesAgo: -10), "Live")
    }
}
