import MonacoCore
import Testing

struct LoginFlowTests {
    private func onCodeStep(destination: String = "+15555550123") -> LoginFlow {
        var flow = LoginFlow()
        let started = flow.beginSend()
        #expect(started)
        flow.sendSucceeded(destination: destination)
        return flow
    }

    @Test func sendingACodeMovesToTheCodeStep() {
        let flow = onCodeStep()
        #expect(flow.isCodeEntry)
        #expect(flow.destination == "+15555550123")
        #expect(flow.phase == .awaitingCode)
    }

    @Test func aThrottledResendKeepsTheCodeStep() {
        var flow = onCodeStep()
        let resent = flow.beginSend()
        #expect(resent)
        flow.sendFailed(message: "Too many attempts. Try again in a minute.")

        #expect(flow.isCodeEntry)
        #expect(flow.destination == "+15555550123")
        #expect(flow.phase == .failed(message: "Too many attempts. Try again in a minute."))
    }

    @Test func sendingStaysOnTheCodeStepSoTheFieldDoesNotFlicker() {
        var flow = onCodeStep()
        let started = flow.beginSend()
        #expect(started)
        #expect(flow.isCodeEntry)
        #expect(flow.phase == .sendingCode)
    }

    @Test func aRejectedOrRateLimitedVerifyKeepsTheCodeStep() {
        var flow = onCodeStep()
        let started = flow.beginVerify()
        #expect(started)
        flow.verifyFailed(message: "That code didn't work. Try again.")

        #expect(flow.isCodeEntry)
        #expect(flow.phase == .failed(message: "That code didn't work. Try again."))
    }

    @Test func aFailedFirstSendStaysOnTheAddressStep() {
        var flow = LoginFlow()
        let started = flow.beginSend()
        #expect(started)
        flow.sendFailed(message: "Couldn't send the code. Try again.")

        #expect(!flow.isCodeEntry)
        #expect(flow.destination == nil)
    }

    @Test func onlyTheMemberGoesBackToTheAddressStep() {
        var flow = onCodeStep()
        flow.returnToAddressEntry()

        #expect(!flow.isCodeEntry)
        #expect(flow.phase == .idle)
    }

    @Test func aSecondTapCannotStartASecondRequest() {
        var flow = LoginFlow()
        let started = flow.beginSend()
        let secondSend = flow.beginSend()
        let verifyWhileSending = flow.beginVerify()

        #expect(started)
        #expect(!secondSend)
        #expect(!verifyWhileSending)
    }

    @Test func aRestoredSessionIsNotSentBackToTheLoginForm() {
        var flow = LoginFlow(phase: .restoring)
        flow.returnToAddressEntry()
        #expect(flow.phase == .restoring)

        flow.authenticated(userID: "user-1")
        flow.returnToAddressEntry()
        #expect(flow.phase == .authenticated(userID: "user-1"))
    }

    @Test func beginVerifyRefusesOnceAuthenticated() {
        var flow = LoginFlow()
        flow.authenticated(userID: "user-1")

        let started = flow.beginVerify()
        #expect(!started)
        #expect(flow.phase == .authenticated(userID: "user-1"))
    }

    @Test func signingOutClearsTheForm() {
        var flow = onCodeStep()
        flow.signedOut()

        #expect(!flow.isCodeEntry)
        #expect(flow.phase == .idle)
    }

    @Test func closingTheSheetReturnsToIdleWithNoToast() {
        var flow = LoginFlow()
        let started = flow.beginAuthorizing(.google)
        #expect(started)
        #expect(flow.phase == .authorizing(.google))
        #expect(flow.isBusy)

        flow.authorizationFailed(.cancelled)

        #expect(flow.phase == .idle)
        #expect(flow.toastMessage == nil)
    }

    @Test func aFailedSheetIsToasted() {
        var flow = LoginFlow()
        _ = flow.beginAuthorizing(.apple)
        flow.authorizationFailed(.other(detail: nil))

        #expect(flow.phase == .providerFailed(.apple, message: "Couldn't sign you in. Try again."))
        #expect(flow.toastMessage == "Couldn't sign you in. Try again.")
    }

    @Test func aSheetToastLeavesOutPrivyCopy() {
        var flow = LoginFlow()
        _ = flow.beginAuthorizing(.google)
        flow.authorizationFailed(.other(detail: "Network Error"))

        #expect(flow.toastMessage == "Couldn't sign you in. Try again.")
    }

    @Test func anOfflineSheetIsToastedAsOffline() {
        var flow = LoginFlow()
        _ = flow.beginAuthorizing(.google)
        flow.authorizationFailed(.offline)

        #expect(flow.toastMessage == "No connection. Check your internet and try again.")
    }

    @Test func aCodeFailureIsNotToasted() {
        var flow = onCodeStep()
        _ = flow.beginVerify()
        flow.verifyFailed(message: "That code didn't work. Try again.")

        #expect(flow.toastMessage == nil)
    }

    @Test func aSecondTapCannotOpenASecondSheet() {
        var flow = LoginFlow()
        _ = flow.beginAuthorizing(.apple)

        let secondSheet = flow.beginAuthorizing(.google)
        let sendWhileUp = flow.beginSend()
        let verifyWhileUp = flow.beginVerify()

        #expect(!secondSheet)
        #expect(!sendWhileUp)
        #expect(!verifyWhileUp)
        #expect(flow.phase == .authorizing(.apple))
    }

    @Test func noSheetOpensWhileACodeIsInFlightOrASessionExists() {
        var sending = LoginFlow()
        _ = sending.beginSend()
        let whileSending = sending.beginAuthorizing(.google)

        var restoring = LoginFlow(phase: .restoring)
        let whileRestoring = restoring.beginAuthorizing(.google)

        var signedIn = LoginFlow()
        signedIn.authenticated(userID: "user-1")
        let whileSignedIn = signedIn.beginAuthorizing(.apple)

        #expect(!whileSending)
        #expect(!whileRestoring)
        #expect(!whileSignedIn)
    }

    @Test func closingTheSheetKeepsACodeTheMemberAlreadyHas() {
        var flow = onCodeStep()
        _ = flow.beginAuthorizing(.google)
        flow.authorizationFailed(.cancelled)

        #expect(flow.isCodeEntry)
        #expect(flow.destination == "+15555550123")
    }

    @Test func aSheetSignInLandsAuthenticated() {
        var flow = LoginFlow()
        _ = flow.beginAuthorizing(.apple)
        flow.authenticated(userID: "user-1")

        #expect(flow.phase == .authenticated(userID: "user-1"))
        #expect(flow.toastMessage == nil)
    }

    @Test func aMissingTokenAfterASheetIsToastedAndAfterACodeIsNot() {
        var sheet = LoginFlow()
        _ = sheet.beginAuthorizing(.google)
        sheet.tokenUnavailable(message: LoginFailureCopy.tokenUnavailable)
        #expect(sheet.toastMessage == LoginFailureCopy.tokenUnavailable)

        var code = onCodeStep()
        _ = code.beginVerify()
        code.tokenUnavailable(message: LoginFailureCopy.tokenUnavailable)
        #expect(code.phase == .failed(message: LoginFailureCopy.tokenUnavailable))
        #expect(code.toastMessage == nil)
    }
}
