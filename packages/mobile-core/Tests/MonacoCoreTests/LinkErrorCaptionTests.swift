import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class LinkErrorCaptionTests: XCTestCase {
    func testEditingTheNumberClearsTheLinkedElsewhereRefusal() async {
        let linking = FakeAccountLinking(sendPhoneCodeError: .alreadyLinkedElsewhere)
        let transport = StubTransport(scripted: [])
        let model = PhoneLinkModel(
            linking: linking, onboarding: OnboardingFlowTests.onboarding(transport), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        XCTAssertTrue(model.linkedElsewhere)
        XCTAssertEqual(model.caption, .error(LinkCopy.phoneLinkedElsewhere))

        model.numberEdited()

        XCTAssertFalse(model.linkedElsewhere)
        XCTAssertNil(model.caption)
    }

    func testAThrottledSendShowsTheSignInThrottleLine() async {
        let linking = FakeAccountLinking(sendPhoneCodeError: .rateLimited)
        let model = PhoneLinkModel(
            linking: linking, onboarding: OnboardingFlowTests.onboarding(StubTransport(scripted: [])),
            clock: TestClock())

        await model.sendCode(to: "+15555550100")

        XCTAssertEqual(model.caption, .error("Too many attempts. Wait a minute, then try again."))
        XCTAssertEqual(model.step, .number)
    }
}
