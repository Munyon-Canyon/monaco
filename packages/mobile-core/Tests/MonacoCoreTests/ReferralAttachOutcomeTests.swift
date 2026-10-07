import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ReferralAttachOutcomeTests: XCTestCase {
    func testCreatedIsAttached() {
        XCTAssertEqual(ReferralAttachOutcome.from(problem: nil), .attached)
    }

    func testAnUnprocessableReferralIsARefusalThatClears() {
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(422, .referralWindowClosed)), .refusedClear)
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(422, .referralSelf)), .refusedClear)
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(422, .referralAlreadyAttached)), .refusedClear)
    }

    func testAnUnknownCodeIsARefusalThatClears() {
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(404, .referralCodeUnknown)), .refusedClear)
    }

    func testARateLimitRetriesLater() {
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(429, .rateLimited)), .retryLater)
    }

    func testAServerErrorRetriesLater() {
        XCTAssertEqual(ReferralAttachOutcome.from(problem: Self.problem(503, .upstreamUnavailable)), .retryLater)
    }

    func testATransportErrorRetriesLater() {
        XCTAssertEqual(
            ReferralAttachOutcome.from(problem: .transport(URLError(.notConnectedToInternet))), .retryLater)
    }

    private static func problem(_ status: Int, _ code: Components.Schemas.ErrorCode) -> APIError {
        .problem(
            ProblemError(
                status: status, code: .known(code), message: "m", traceID: "t", retryable: status >= 429))
    }
}
