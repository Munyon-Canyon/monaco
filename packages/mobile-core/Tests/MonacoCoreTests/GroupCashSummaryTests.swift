import MonacoAPI
import MonacoCore
import XCTest

final class GroupCashSummaryTests: XCTestCase {
    func testTodaysPotPayloadAwaitsLiveConfirmationAndNeverShowsTheConstantRate() {
        let summary = GroupCashSummary(CabalPotSummary(.sampleCashOnly))

        XCTAssertEqual(summary.yieldStatus, .awaitingConfirmation)
        XCTAssertEqual(summary.yieldLabel, "Rate requires live confirmation")
        XCTAssertFalse(summary.yieldLabel.contains("APY"))
        XCTAssertEqual(summary.asset, "USDC")
    }

    func testTheCashFigureIsThePotsCash() {
        let pot = CabalPotSummary(.sampleCashOnly)

        XCTAssertEqual(GroupCashSummary(pot).cash, pot.cash)
    }

    func testAwaitingAndUnavailableKeepTheNoticeInInfoAndWarnTones() {
        let pot = CabalPotSummary(.sampleCashOnly)

        XCTAssertEqual(GroupCashSummary(pot).notice?.tone, .brand)
        XCTAssertEqual(GroupCashSummary(pot, yieldStatus: .unavailable).notice?.tone, .warn)
        XCTAssertEqual(GroupCashSummary(pot, yieldStatus: .unavailable).yieldLabel, "Rate unavailable")
    }

    func testARateFromTheAPIReplacesTheLabelAndDropsTheNotice() {
        let summary = GroupCashSummary(CabalPotSummary(.sampleCashOnly), yieldStatus: .rate("4.1% APY"))

        XCTAssertEqual(summary.yieldLabel, "4.1% APY")
        XCTAssertNil(summary.notice)
    }
}
