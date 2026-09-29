import XCTest
@testable import MonacoCore

final class LoginPhaseTests: XCTestCase {
    func testCancellingTheSheetReturnsToIdleWithNoToast() {
        var phase = LoginPhase.idle
        XCTAssertTrue(phase.beginAuthorizing(.google))
        XCTAssertEqual(phase, .authorizing(.google))

        phase.authorizationFailed(.cancelled)

        XCTAssertEqual(phase, .idle)
        XCTAssertNil(phase.toastMessage)
    }

    func testAFailedSignInToastsItsCopy() {
        var phase = LoginPhase.idle
        XCTAssertTrue(phase.beginAuthorizing(.apple))

        phase.authorizationFailed(.offline)

        XCTAssertEqual(phase.toastMessage, "No connection. Check your internet and try again.")
    }

    func testAProviderErrorToastsTheGenericSignInCopyWithItsDetail() {
        var phase = LoginPhase.idle
        XCTAssertTrue(phase.beginAuthorizing(.google))

        phase.authorizationFailed(.other(detail: "Provider unavailable."))

        XCTAssertEqual(phase.toastMessage, "Couldn't sign you in. Try again. Provider unavailable.")
    }

    func testASecondTapCannotOpenASecondSheet() {
        var phase = LoginPhase.idle
        XCTAssertTrue(phase.beginAuthorizing(.apple))
        XCTAssertFalse(phase.beginAuthorizing(.google))
        XCTAssertEqual(phase, .authorizing(.apple))
    }

    func testTheMemberCanTryAgainAfterAFailure() {
        var phase = LoginPhase.failed(message: "No connection. Check your internet and try again.")
        XCTAssertTrue(phase.beginAuthorizing(.google))
        XCTAssertNil(phase.toastMessage)
    }

    func testARestoringOrSignedInSessionIgnoresTheButtons() {
        var restoring = LoginPhase.restoring
        XCTAssertFalse(restoring.beginAuthorizing(.apple))
        XCTAssertEqual(restoring, .restoring)

        var signedIn = LoginPhase.authenticated(userID: "user-1")
        XCTAssertFalse(signedIn.beginAuthorizing(.apple))
        XCTAssertEqual(signedIn, .authenticated(userID: "user-1"))
    }

    func testOnlyAFailureToasts() {
        let quiet: [LoginPhase] = [
            .restoring,
            .restoreFailed(message: "offline"),
            .idle,
            .authorizing(.apple),
            .authenticated(userID: "user-1"),
        ]
        for phase in quiet {
            XCTAssertNil(phase.toastMessage, "\(phase)")
        }
    }
}

/// The rule under test: a failed request never takes the code field away. Only the member asking
/// to change their number goes back to the address step.
final class OTPFlowTests: XCTestCase {
    private func onCodeStep(destination: String = "+15555550123") -> OTPFlow {
        var flow = OTPFlow()
        XCTAssertTrue(flow.beginSend())
        flow.sendSucceeded(destination: destination)
        return flow
    }

    func testSendingACodeMovesToTheCodeStep() {
        let flow = onCodeStep()
        XCTAssertTrue(flow.isCodeEntry)
        XCTAssertEqual(flow.destination, "+15555550123")
        XCTAssertEqual(flow.phase, .awaitingCode)
    }

    func testAThrottledResendKeepsTheCodeStep() {
        var flow = onCodeStep()
        XCTAssertTrue(flow.beginSend())
        XCTAssertTrue(flow.isCodeEntry, "sending stays on the code step so the field does not flicker")
        flow.sendFailed(message: "Too many attempts. Try again in a minute.")

        XCTAssertTrue(flow.isCodeEntry)
        XCTAssertEqual(flow.destination, "+15555550123")
        XCTAssertEqual(flow.phase, .failed(message: "Too many attempts. Try again in a minute."))
    }

    func testARejectedVerifyKeepsTheCodeStep() {
        var flow = onCodeStep()
        XCTAssertTrue(flow.beginVerify())
        flow.verifyFailed(message: "That code didn't work. Try again.")

        XCTAssertTrue(flow.isCodeEntry)
        XCTAssertEqual(flow.phase, .failed(message: "That code didn't work. Try again."))
    }

    func testAFailedFirstSendStaysOnTheAddressStep() {
        var flow = OTPFlow()
        XCTAssertTrue(flow.beginSend())
        flow.sendFailed(message: "Couldn't send the code. Try again.")

        XCTAssertFalse(flow.isCodeEntry)
        XCTAssertNil(flow.destination)
    }

    func testOnlyTheMemberGoesBackToTheAddressStep() {
        var flow = onCodeStep()
        flow.returnToAddressEntry()

        XCTAssertFalse(flow.isCodeEntry)
        XCTAssertEqual(flow.phase, .idle)
    }

    func testASecondTapCannotStartASecondRequest() {
        var flow = OTPFlow()
        XCTAssertTrue(flow.beginSend())
        XCTAssertFalse(flow.beginSend())
        XCTAssertFalse(flow.beginVerify())
    }

    func testAnAcceptedCodeHoldsTheForm() {
        var flow = onCodeStep()
        XCTAssertTrue(flow.beginVerify())
        flow.verified()

        XCTAssertTrue(flow.isBusy)
        XCTAssertFalse(flow.beginSend())
        flow.returnToAddressEntry()
        XCTAssertTrue(flow.isCodeEntry)
        XCTAssertEqual(flow.phase, .verified)
    }
}
