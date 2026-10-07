import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

extension OnboardingFlowTests {
    func testAnSmsSignUpConfirmsItsNumberWithoutAskingForOneOrSendingACode() async throws {
        let linking = FakeAccountLinking()
        let transport = StubTransport(.json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)))
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        let result = await model.confirmSignInPhone()

        let profile = try XCTUnwrap(result.finishedProfile)
        XCTAssertEqual(profile.authState, .awaitingSocials)
        XCTAssertFalse(model.signInPhoneUnavailable)
        let calls = await linking.calls
        XCTAssertEqual(calls, [])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/onboarding/phone"])
    }

    func testASignInNumberAnotherUserHoldsFallsBackToTheForm() async throws {
        let refusal = try StubTransport.Reply.problem(
            Self.problem(code: .phoneNotLinked, message: "Link a phone first."))
        let transport = StubTransport(scripted: [refusal])
        let linking = FakeAccountLinking()
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        let result = await model.confirmSignInPhone()

        XCTAssertEqual(result, .stay)
        XCTAssertTrue(model.signInPhoneUnavailable)
        XCTAssertNil(model.caption)
        let calls = await linking.calls
        XCTAssertEqual(calls, [])
    }

    func testAFailedSignInNumberConfirmationFallsBackToTheFormWithACaption() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))])
        let model = PhoneLinkModel(
            linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: TestClock())

        let result = await model.confirmSignInPhone()

        XCTAssertEqual(result, .stay)
        XCTAssertTrue(model.signInPhoneUnavailable)
        XCTAssertNotNil(model.caption)
    }

    func testANumberTheAccountAlreadyHasShowsAClearCaption() async {
        let linking = FakeAccountLinking(linkPhoneError: .alreadyHasPhone)
        let model = PhoneLinkModel(
            linking: linking, onboarding: Self.onboarding(StubTransport(scripted: [])), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "123456"
        _ = await model.link()

        XCTAssertEqual(model.caption, .error(LinkCopy.phoneAlreadyOnAccount))
        XCTAssertFalse(model.linkedElsewhere)
    }
}
