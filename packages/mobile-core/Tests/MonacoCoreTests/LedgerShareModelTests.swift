import Foundation
import MonacoAPI
import MonacoRanking
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class LedgerShareModelTests: XCTestCase {
    func testTheLedgerSubjectDefaultsToTheStoryWithEveryEyeOn() {
        let defaults = ShareScreenModel.defaults(for: .ledger(week: "2026-W42"))

        XCTAssertEqual(defaults.format, .story)
        XCTAssertEqual(defaults.eyes, ShareEyes())
    }
}
