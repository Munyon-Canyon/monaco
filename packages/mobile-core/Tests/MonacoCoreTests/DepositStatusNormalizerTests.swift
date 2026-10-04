import XCTest

@testable import MonacoCore

final class DepositStatusNormalizerTests: XCTestCase {
    func testDepositStatusNormalizer_failedPrefix() {
        XCTAssertTrue(DepositStatusNormalizer.isFailed("failed: submit_sweep"))
        XCTAssertFalse(DepositStatusNormalizer.isFailed("pending"))
    }

    func testDepositStatusNormalizer_pendingAndConfirmedIgnoreCase() {
        XCTAssertTrue(DepositStatusNormalizer.isPending("Pending"))
        XCTAssertFalse(DepositStatusNormalizer.isPending("confirmed"))
        XCTAssertTrue(DepositStatusNormalizer.isConfirmed("CONFIRMED"))
        XCTAssertFalse(DepositStatusNormalizer.isConfirmed("failed"))
    }
}
