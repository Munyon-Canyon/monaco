import MonacoAPI
import MonacoCore
import XCTest

final class ProposalChipTests: XCTestCase {
    func testEveryStatusForBuyAndSell() {
        let expected: [ProposalStatus: (buy: String?, sell: String?)] = [
            .open: (nil, nil),
            .passed: ("Buying", "Selling"),
            .executed: ("Bought", "Sold"),
            .failed: ("Didn't pass", "Didn't pass"),
            .expired: ("Expired", "Expired"),
            .withdrawn: ("Withdrawn", "Withdrawn"),
            .voided: ("Voided by Monaco", "Voided by Monaco"),
            .executionBlocked: ("Couldn't buy", "Couldn't sell"),
        ]
        XCTAssertEqual(expected.count, ProposalStatus.allCases.count)
        for status in ProposalStatus.allCases {
            XCTAssertEqual(ProposalChip.label(status: status, kind: .buy), expected[status]?.buy, "\(status)")
            XCTAssertEqual(ProposalChip.label(status: status, kind: .sell), expected[status]?.sell, "\(status)")
        }
    }

    func testEveryWireStatusAndKindMapsToItsOwnCase() {
        let statuses = Components.Schemas.ProposalStatus.allCases.map { ProposalStatus($0).rawValue }
        XCTAssertEqual(statuses, Components.Schemas.ProposalStatus.allCases.map(\.rawValue))
        XCTAssertEqual(Set(statuses).count, ProposalStatus.allCases.count)
        XCTAssertEqual(ProposalKind(.buy), .buy)
        XCTAssertEqual(ProposalKind(.sell), .sell)
        XCTAssertEqual(ProposalKind.buy.title, "Buy")
        XCTAssertEqual(ProposalKind.sell.title, "Sell")
    }

    func testAFailedSwapOnAPassedProposalCouldNotTrade() {
        XCTAssertEqual(ProposalChip.label(status: .passed, kind: .buy, swap: .failed), "Couldn't buy")
        XCTAssertEqual(ProposalChip.label(status: .passed, kind: .sell, swap: .failed), "Couldn't sell")
        XCTAssertEqual(ProposalChip.label(status: .passed, kind: .buy, swap: .submitted), "Buying")
    }

    func testStepsReachTheStepTheStatusIsAt() {
        XCTAssertEqual(steps(.open).reached, 0)
        XCTAssertEqual(steps(.passed).reached, 1)
        XCTAssertEqual(steps(.passed, swap: .confirmed).reached, 2)
        XCTAssertEqual(steps(.executed).reached, 2)
        XCTAssertEqual(steps(.expired).reached, 0)
        XCTAssertEqual(steps(.executed).titles, ["Voting", "Buying", "Done"])
        XCTAssertEqual(steps(.executed, kind: .sell).titles, ["Voting", "Selling", "Done"])
        XCTAssertFalse(steps(.executed).failed)
    }

    func testABlockedOrFailedTradeEndsOnCouldNotTrade() {
        let blocked = steps(.executionBlocked, message: "The pot is short.")
        XCTAssertEqual(blocked.titles, ["Voting", "Buying", "Couldn't buy"])
        XCTAssertEqual(blocked.reached, 2)
        XCTAssertTrue(blocked.failed)
        XCTAssertEqual(blocked.failureMessage, "The pot is short.")
        let failedSell = steps(.passed, kind: .sell, swap: .failed, message: "No route.")
        XCTAssertEqual(failedSell.titles.last, "Couldn't sell")
        XCTAssertEqual(failedSell.failureMessage, "No route.")
    }

    private func steps(
        _ status: ProposalStatus, kind: ProposalKind = .buy, swap: SwapState? = nil, message: String? = nil
    ) -> ProposalSteps {
        ProposalSteps(status: status, kind: kind, swap: swap, failureMessage: message)
    }
}
