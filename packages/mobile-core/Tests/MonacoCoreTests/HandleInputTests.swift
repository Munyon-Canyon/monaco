import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class HandleInputTests: XCTestCase {
    func testNormalizeTrimsLowercasesAndDropsOneLeadingAt() {
        XCTAssertEqual(HandleInput.normalize("  QA_Handle_1 \n"), "qa_handle_1")
        XCTAssertEqual(HandleInput.normalize("@KaiCenat"), "kaicenat")
        XCTAssertEqual(HandleInput.normalize(" @kai"), "kai")
        XCTAssertEqual(HandleInput.normalize("@@kai"), "@kai")
        XCTAssertEqual(HandleInput.normalize("@"), "")
    }

    func testLocalReasonAcceptsThreeToTwentyHandleCharacters() {
        XCTAssertNil(HandleInput.localReason("abc"))
        XCTAssertNil(HandleInput.localReason(String(repeating: "a", count: 20)))
        XCTAssertNil(HandleInput.localReason("qa_handle_1"))
        XCTAssertNil(HandleInput.localReason(HandleInput.normalize("@QA_Handle_1")))
    }

    func testLocalReasonRefusesBoundaryLengthsAndOtherCharacters() {
        XCTAssertEqual(HandleInput.localReason(""), .invalid)
        XCTAssertEqual(HandleInput.localReason("ad"), .invalid)
        XCTAssertEqual(HandleInput.localReason(String(repeating: "a", count: 21)), .invalid)
        XCTAssertEqual(HandleInput.localReason("QA_handle"), .invalid, "expects a normalized handle")
        XCTAssertEqual(HandleInput.localReason("qa-handle"), .invalid)
        XCTAssertEqual(HandleInput.localReason("qa handle"), .invalid)
        XCTAssertEqual(HandleInput.localReason("@qa"), .invalid)
    }

    func testLocalReasonRefusesUnicodeTheServerWouldRefuse() {
        XCTAssertEqual(HandleInput.localReason(HandleInput.normalize("José")), .invalid)
        XCTAssertEqual(HandleInput.localReason("kai🙂"), .invalid)
        XCTAssertEqual(HandleInput.localReason("ｋａｉ"), .invalid, "fullwidth letters are not a-z")
        XCTAssertEqual(
            HandleInput.localReason("ab\u{301}"), .invalid,
            "a combining mark is its own scalar, so the length and charset are counted per scalar")
    }

    func testReasonCopy() {
        XCTAssertEqual(HandleReason.taken.message(changeableAt: nil), "That handle is taken.")
        XCTAssertEqual(HandleReason.reserved.message(changeableAt: nil), "That handle isn't available.")
        XCTAssertEqual(
            HandleReason.invalid.message(changeableAt: nil), "Use 3 to 20 letters, numbers or underscores.")
    }

    func testTooSoonNamesTheDateTheHandleCanChange() throws {
        let changeableAt = try XCTUnwrap(SharedFormatters.iso8601Date(from: "2026-11-03T12:00:00Z"))
        let utc = try XCTUnwrap(TimeZone(identifier: "UTC"))

        XCTAssertEqual(
            HandleReason.tooSoon.message(changeableAt: changeableAt, timeZone: utc),
            "You can change your handle again on November 3, 2026.")
    }

    func testStepReasonIsFirstRunOnlyForANewUser() {
        XCTAssertEqual(HandleInput.stepReason(for: Self.profile(.created)), .firstRun)
        XCTAssertEqual(HandleInput.stepReason(for: Self.profile(.awaitingPhone)), .revoked)
        XCTAssertEqual(HandleInput.stepReason(for: Self.profile(.awaitingSocials)), .revoked)
        XCTAssertEqual(HandleInput.stepReason(for: Self.profile(.onboardingCompleted)), .revoked)
    }

    func testStepReasonPicksTheSubtext() {
        XCTAssertEqual(HandleStepReason.firstRun.subtext, "This is how people find you on Monaco.")
        XCTAssertEqual(HandleStepReason.revoked.subtext, "Your handle was removed. Pick a new one.")
    }

    func testOnlyAnAvailableStatusIsClaimable() {
        XCTAssertEqual(HandleStatus.available("kai").claimable, "kai")
        for status in [
            HandleStatus.idle, .checking("kai"), .unavailable("kai", .taken), .slowDown("kai"), .failed("kai"),
        ] {
            XCTAssertNil(status.claimable, "\(status)")
        }
    }

    func testASaveRaceStaysInlineAndOtherFailuresToast() {
        XCTAssertEqual(HandleSaveFailure(.problem(Self.problem("handle_taken", 409))), .inline(.taken))
        XCTAssertEqual(HandleSaveFailure(.problem(Self.problem("handle_too_soon", 409))), .inline(.tooSoon))
        XCTAssertEqual(
            HandleSaveFailure(.transport(URLError(.notConnectedToInternet))), .toast("You're offline. Try again."))
        XCTAssertEqual(
            HandleSaveFailure(.problem(Self.problem("internal", 500, "Monaco hit a snag."))),
            .toast("Monaco hit a snag."))
    }

    private static func problem(_ code: String, _ status: Int, _ message: String = "No.") -> ProblemError {
        ProblemError(status: status, code: .init(code), message: message, traceID: "trace", retryable: false)
    }

    private static func profile(_ authState: SessionProfile.AuthState) -> SessionProfile {
        SessionProfile(
            userID: "u1", handle: nil, displayName: "Kai", photoURL: nil, authState: authState,
            accountStatus: .active, loginProvider: .email, memberWalletAddress: "wallet-1", phoneLinked: false,
            xUsername: nil,
            handleChangeableAt: nil, createdAt: Date())
    }
}
