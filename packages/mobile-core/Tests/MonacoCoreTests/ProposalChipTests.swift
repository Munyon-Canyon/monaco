import MonacoCore
import XCTest

final class ProposalChipTests: XCTestCase {
    func testEveryStatusForBuyAndSell() {
        let expected: [ProposalStatus: (String?, String?)] = [
            .open: (nil, nil), .passed: ("Buying", "Selling"), .executed: ("Bought", "Sold"),
            .failed: ("Didn't pass", "Didn't pass"), .expired: ("Expired", "Expired"),
            .withdrawn: ("Withdrawn", "Withdrawn"), .voided: ("Voided by Monaco", "Voided by Monaco"),
            .executionBlocked: ("Couldn't buy", "Couldn't sell"),
        ]
        for status in ProposalStatus.allCases {
            XCTAssertEqual(ProposalChip.label(status: status, isSell: false), expected[status]?.0)
            XCTAssertEqual(ProposalChip.label(status: status, isSell: true), expected[status]?.1)
        }
    }

    func testFailedSwapOverridesPassed() {
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: false, swapFailed: true), "Couldn't buy")
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: true, swapFailed: true), "Couldn't sell")
    }
}
