import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class HomeErrorPlanTests: XCTestCase {
    private static func plan(_ statuses: [HomeReadSlot: HomeReadStatus]) -> HomeErrorPlan {
        var plan = HomeErrorPlan()
        for (slot, status) in statuses { plan.report(slot, status) }
        return plan
    }

    func testEveryReadOfflineShowsOneSharedRowAndNoSlotRow() {
        let plan = Self.plan([.portfolio: .offline, .balance: .offline, .board: .offline])

        XCTAssertTrue(plan.showsSharedRow)
        XCTAssertTrue(HomeReadSlot.allCases.allSatisfy { !plan.showsOwnRow($0) })
    }

    func testOneReadFailingWhileOthersLoadShowsOnlyThatSlotsRow() {
        let plan = Self.plan([.portfolio: .settled, .balance: .offline, .board: .pending])

        XCTAssertFalse(plan.showsSharedRow)
        XCTAssertEqual(HomeReadSlot.allCases.filter(plan.showsOwnRow), [.balance])
    }

    func testOneReadFailingWhileOthersSucceedShowsOnlyThatSlotsRow() {
        let plan = Self.plan([.portfolio: .failed, .balance: .settled, .board: .settled])

        XCTAssertFalse(plan.showsSharedRow)
        XCTAssertEqual(HomeReadSlot.allCases.filter(plan.showsOwnRow), [.portfolio])
    }

    func testDifferentFailuresKeepTheirOwnRows() {
        let plan = Self.plan([.portfolio: .offline, .balance: .failed, .board: .offline])

        XCTAssertFalse(plan.showsSharedRow)
        XCTAssertTrue(HomeReadSlot.allCases.allSatisfy { plan.showsOwnRow($0) })
    }

    func testASlotThatHasNotReportedBlocksTheSharedRow() {
        let plan = Self.plan([.portfolio: .offline, .balance: .offline])

        XCTAssertFalse(plan.showsSharedRow)
    }

    func testTransportErrorsReadAsOfflineAndOthersAsFailed() {
        XCTAssertEqual(
            HomeReadStatus(LoadState<Int>.failed(.transport(URLError(.notConnectedToInternet)))), .offline)
        XCTAssertEqual(HomeReadStatus(LoadState<Int>.failed(.decoding("bad"))), .failed)
        XCTAssertEqual(HomeReadStatus(LoadState<Int>.loading), .pending)
        XCTAssertEqual(HomeReadStatus(LoadState<Int>.loaded(1)), .settled)
        XCTAssertEqual(HomeReadStatus(LeaderboardLoader.Phase.failed(.transport(URLError(.timedOut)))), .offline)
        XCTAssertEqual(HomeReadStatus(LeaderboardLoader.Phase.empty), .settled)
    }
}
