import Foundation
import MonacoCore
import XCTest

final class BalanceChangeTests: XCTestCase {
    func testARiseWithNothingInFlightChangingIsADeposit() {
        XCTAssertEqual(
            BalanceChange.detect(previous: balance(12_000_000), current: balance(17_000_000)), .deposited(5_000_000))
    }

    func testADepositWhileAFundIsInFlightStillCounts() {
        XCTAssertEqual(
            BalanceChange.detect(
                previous: balance(7_000_000, inFlight: 5_000_000), current: balance(9_000_000, inFlight: 5_000_000)),
            .deposited(2_000_000)
        )
    }

    func testAFailedFundReturningItsMoneyIsNotADeposit() {
        XCTAssertNil(
            BalanceChange.detect(previous: balance(7_000_000, inFlight: 5_000_000), current: balance(12_000_000)))
    }

    func testAnUnchangedOrFallingBalanceIsNotADeposit() {
        XCTAssertNil(BalanceChange.detect(previous: balance(12_000_000), current: balance(12_000_000)))
        XCTAssertNil(BalanceChange.detect(previous: balance(12_000_000), current: balance(7_000_000)))
    }

    func testTheFirstReadIsNotADeposit() {
        XCTAssertNil(BalanceChange.detect(previous: nil, current: balance(12_000_000)))
    }

    func testADepositSaysHowMuchArrived() {
        XCTAssertEqual(BalanceChange.deposited(25_000_000).message, "Deposit received: $25.00")
        XCTAssertEqual(BalanceChange.deposited(1_234_560_000).message, "Deposit received: $1,234.56")
    }

    private func balance(_ available: Int64, inFlight: Int64 = 0) -> AccountBalance {
        AccountBalance(
            availableMicros: available, onChainMicros: available + inFlight, inFlightMicros: inFlight,
            depositAddress: "wallet-1", asOf: Date(timeIntervalSince1970: 1_759_579_200)
        )
    }
}
