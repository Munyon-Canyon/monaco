import XCTest

@testable import MonacoCore

final class ProposalStepperTests: XCTestCase {
    func testMapsEveryStatus() {
        XCTAssertEqual(ProposalStepper.state(status: .open, isSell: false), .voting)
        XCTAssertEqual(ProposalStepper.state(status: .passed, isSell: false), .trading)
        XCTAssertEqual(ProposalStepper.state(status: .executed, isSell: false), .done)
        XCTAssertEqual(ProposalStepper.state(status: .failed, isSell: false), .failed("Didn't pass"))
        XCTAssertEqual(ProposalStepper.state(status: .expired, isSell: false), .failed("Expired"))
        XCTAssertEqual(ProposalStepper.state(status: .withdrawn, isSell: false), .failed("Withdrawn"))
        XCTAssertEqual(ProposalStepper.state(status: .voided, isSell: false), .failed("Voided by Monaco"))
        XCTAssertEqual(ProposalStepper.state(status: .executionBlocked, isSell: true), .failed("Couldn't sell"))
    }

    func testFailedSwapWinsOverPassed() {
        XCTAssertEqual(ProposalStepper.state(status: .passed, isSell: false, swapFailed: true), .failed("Couldn't buy"))
    }
}
