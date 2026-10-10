import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

extension OnboardingFlowTests {
    func testANumberTheBackendSaysAnotherAccountHoldsStaysWithTheAppLineAfterOneRequest() async throws {
        let refusal = try StubTransport.Reply.problem(
            Self.problem(code: .phoneLinkedElsewhere, message: "Server wording."))
        let transport = StubTransport(scripted: [refusal])
        let clock = TestClock()
        let model = PhoneLinkModel(linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: clock)
        await model.sendCode(to: "+15555550100")
        model.code = "123456"

        let result = await model.link()

        XCTAssertEqual(result, .stay)
        XCTAssertEqual(model.caption, .error(LinkCopy.phoneLinkedElsewhere))
        XCTAssertTrue(model.linkedElsewhere)
        XCTAssertEqual(clock.state.current.requested, [], "a held number is a final answer, so nothing waits to retry")
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/onboarding/phone"])
    }

    func testAnXAccountTheBackendSaysAnotherAccountHoldsStaysWithTheAppLineAfterOneRequest() async throws {
        let refusal = try StubTransport.Reply.problem(
            Self.problem(code: .xLinkedElsewhere, message: "Server wording."))
        let transport = StubTransport(scripted: [refusal])
        let clock = TestClock()
        let model = XLinkModel(linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: clock)

        let result = await model.connect()

        XCTAssertEqual(result, .stay)
        XCTAssertEqual(model.caption, .error(LinkCopy.xLinkedElsewhere))
        XCTAssertTrue(model.linkedElsewhere)
        XCTAssertTrue(model.connectHidden)
        XCTAssertEqual(clock.state.current.requested, [], "a held account is a final answer, so nothing waits to retry")
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/onboarding/socials"])
    }
}
