import Foundation
import MonacoCore
import Testing

@testable import Monaco

/// What the form says besides its helper: whichever of the error, the in-flight note and the
/// address hint is most pressing, and never two notices at once.
@MainActor
struct OTPFieldCaptionTests {
    private let hint = OTPDestination.sms.invalidHint

    private func caption(
        _ phase: LoginPhase,
        codeStep: Bool = false,
        invalid: Bool = false,
        resent: Bool = false
    ) -> OTPFieldCaption? {
        OTPFieldCaption.resolve(
            phase: phase,
            isCodeStep: codeStep,
            invalidHint: invalid ? hint : nil,
            resent: resent
        )
    }

    @Test func anUntouchedAddressStepSaysNothingBesidesItsHelper() {
        #expect(caption(.idle) == nil)
    }

    @Test func aSendInFlightIsAnInfoNotice() {
        let sending = caption(.sendingCode)
        #expect(sending == .note(OTPFieldCaption.sendingCode))
        #expect(sending?.tone == .info)
    }

    @Test func aNumberThatCannotBeTextedGetsTheHintAboveTheHelper() {
        #expect(caption(.idle, invalid: true) == .hint(hint))
        #expect(caption(.idle, invalid: true)?.tone == nil)
    }

    @Test func aFailureWinsOnEitherStep() {
        let message = LoginFailureCopy.message(for: .rateLimited, step: .sendCode)
        #expect(caption(.failed(message: message), invalid: true) == .error(message))
        #expect(caption(.failed(message: message), codeStep: true, resent: true) == .error(message))
        #expect(caption(.failed(message: message))?.tone == .error)
    }

    @Test func theCodeStepIsQuietUntilThereIsSomethingToSay() {
        #expect(caption(.awaitingCode, codeStep: true) == nil)
    }

    @Test func aCodeBeingCheckedIsAnInfoNotice() {
        #expect(caption(.verifyingCode, codeStep: true) == .note(OTPFieldCaption.verifyingCode))
        #expect(caption(.verifyingCode, codeStep: true)?.tone == .info)
    }

    @Test func aResendIsSaidWhileItRunsAndOnceItLands() {
        #expect(caption(.sendingCode, codeStep: true) == .note(OTPFieldCaption.sendingNewCode))
        #expect(caption(.awaitingCode, codeStep: true, resent: true) == .note(OTPFieldCaption.newCodeSent))
    }
}

/// Which layout the login screen draws, read off the flow.
@MainActor
struct LoginFlowLayoutTests {
    private func codeStep(sentTo address: String = "+15551234567") -> LoginFlow {
        var flow = LoginFlow()
        _ = flow.beginSend()
        flow.sendSucceeded(destination: address)
        return flow
    }

    @Test func theAddressStepIsNotTheCodeStep() {
        #expect(!LoginFlow().isOnCodeStep)
    }

    @Test func aSentCodeAndASignedInMemberAreBothOnTheCodeStep() {
        var flow = codeStep()
        #expect(flow.isOnCodeStep)
        flow.authenticated(userID: "user")
        #expect(flow.isOnCodeStep)
    }

    @Test func onlyAnOfflineFailureDrawsTheTryAgainState() {
        var offline = LoginFlow()
        offline.sendFailed(message: OTPCode.offlineMessage)
        #expect(offline.isOffline)

        var throttled = LoginFlow()
        throttled.sendFailed(message: LoginFailureCopy.message(for: .rateLimited, step: .sendCode))
        #expect(!throttled.isOffline)

        var rejected = codeStep()
        _ = rejected.beginVerify()
        rejected.verifyFailed(message: OTPCode.rejectedMessage)
        #expect(!rejected.isOffline)
    }

    @Test func offlineOnTheCodeStepKeepsTheCodeStep() {
        var flow = codeStep()
        _ = flow.beginVerify()
        flow.verifyFailed(message: OTPCode.offlineMessage)
        #expect(flow.isOffline)
        #expect(flow.isOnCodeStep)
    }

    @Test func tryAgainReturnsToTheStepTheFailureLeft() {
        var address = LoginFlow()
        address.sendFailed(message: OTPCode.offlineMessage)
        address.clearFailure()
        #expect(!address.isOffline)
        #expect(!address.isOnCodeStep)

        var code = codeStep()
        _ = code.beginVerify()
        code.verifyFailed(message: OTPCode.offlineMessage)
        code.clearFailure()
        #expect(!code.isOffline)
        #expect(code.isOnCodeStep)
    }
}

/// The button says what it will do, or what it is doing.
@MainActor
struct OTPPrimaryActionTests {
    @Test func theAddressStepSendsACode() {
        #expect(OTPPrimaryAction.title(phase: .idle, isCodeStep: false) == "Send code")
        #expect(OTPPrimaryAction.title(phase: .sendingCode, isCodeStep: false) == "Sending\u{2026}")
    }

    @Test func theCodeStepContinues() {
        #expect(OTPPrimaryAction.title(phase: .awaitingCode, isCodeStep: true) == "Continue")
        #expect(OTPPrimaryAction.title(phase: .verifyingCode, isCodeStep: true) == "Verifying\u{2026}")
    }

    /// A new code on its way is not the member signing in.
    @Test func aResendDoesNotRenameTheButton() {
        #expect(OTPPrimaryAction.title(phase: .sendingCode, isCodeStep: true) == "Continue")
    }
}

/// How the number a code went to reads back.
@MainActor
struct PhoneReadBackTests {
    @Test func aNorthAmericanNumberReadsBackInTheShapeTheHintAsksFor() {
        #expect(PhoneReadBack.format("+15551234567") == "+1 555 123 4567")
    }

    @Test func whatAutofillHandsTheFieldReadsBackTheSameWay() throws {
        let parsed = try #require(E164PhoneNumber("+1 (555) 123-4567"))
        #expect(PhoneReadBack.format(parsed.value) == "+1 555 123 4567")
    }

    /// Where the spaces go depends on the country, which the app does not look up.
    @Test func otherCountriesReadBackAsSent() {
        #expect(PhoneReadBack.format("+447700900123") == "+447700900123")
    }

    @Test func anythingElseComesBackUntouched() {
        #expect(PhoneReadBack.format("+1555") == "+1555")
        #expect(PhoneReadBack.format("") == "")
    }
}

/// Which ways in the form offers.
@MainActor
struct LoginMethodTests {
    @Test func textingComesFirstWhenBothAreOn() {
        #expect(LoginMethod.available(sms: true, email: true) == [.sms, .email])
    }

    @Test func onlyTheMethodsTurnedOnAreOffered() {
        #expect(LoginMethod.available(sms: false, email: true) == [.email])
        #expect(LoginMethod.available(sms: true, email: false) == [.sms])
        #expect(LoginMethod.available(sms: false, email: false).isEmpty)
    }
}

/// The words on the way in.
@MainActor
struct WelcomeCopyTests {
    /// Nothing on the way in says wallet, group, treasury or the rest of the banned list.
    @Test func theWayInUsesTheProductsWords() {
        let strings = [
            SessionGateCopy.restoreFailedTitle,
            SessionGateCopy.openFailedTitle,
            OTPFieldCaption.sendingCode,
            OTPFieldCaption.verifyingCode,
            OTPFieldCaption.sendingNewCode,
            OTPFieldCaption.newCodeSent,
            OTPCode.offlineMessage,
            OTPDestination.sms.caption,
            OTPDestination.sms.placeholder,
            OTPDestination.sms.invalidHint,
            OTPDestination.sms.changeLabel,
            OTPDestination.sms.verifyTitle,
            OTPDestination.email.caption,
            OTPDestination.email.placeholder,
            OTPDestination.email.invalidHint,
            OTPDestination.email.changeLabel,
            OTPDestination.email.verifyTitle,
            OTPPrimaryAction.title(phase: .idle, isCodeStep: false),
            OTPPrimaryAction.title(phase: .sendingCode, isCodeStep: false),
            OTPPrimaryAction.title(phase: .awaitingCode, isCodeStep: true),
            OTPPrimaryAction.title(phase: .verifyingCode, isCodeStep: true),
        ]
        #expect(MainFlowCopyAudit.stringsAreClean(strings))
    }

    /// The gate's title and the generic message under it used to be the same sentence.
    @Test func theGateTitleDoesNotRepeatTheMessageUnderIt() throws {
        struct SomethingElse: Error {}
        let base = try #require(URL(string: "https://example.com"))
        let generic = SessionErrorMapping.describe(SomethingElse(), apiBaseURL: base)
        #expect(generic.message == "Couldn't open Monaco. Try again.")
        #expect(!generic.message.hasPrefix(SessionGateCopy.openFailedTitle))
    }
}
