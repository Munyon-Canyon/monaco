import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class OnboardingFlowTests: XCTestCase {
    func testLinkingAPhoneMovesTheGateToTheXStep() async throws {
        let linking = FakeAccountLinking()
        let transport = StubTransport(.json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)))
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "123456"
        let result = await model.link()

        let profile = try XCTUnwrap(result.finishedProfile)
        XCTAssertEqual(profile.authState, .awaitingSocials)
        XCTAssertEqual(
            FirstRunGate.destination(for: profile, onboardingCursor: OnboardingCursor.start.advanced(past: .phone)),
            .socials)
        let calls = await linking.calls
        XCTAssertEqual(calls, [.sendPhoneCode("+15555550100"), .linkPhone("123456")])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/onboarding/phone"])
        XCTAssertNotNil(sent.first?.headerFields[try Self.keyHeader()])
    }

    func testANumberLinkedElsewhereNeverReachesTheBackend() async {
        let linking = FakeAccountLinking(linkPhoneError: .alreadyLinkedElsewhere)
        let transport = StubTransport(scripted: [])
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "123456"
        let result = await model.link()

        XCTAssertEqual(result, .stay)
        XCTAssertTrue(model.linkedElsewhere)
        XCTAssertEqual(model.caption, .error(LinkCopy.phoneLinkedElsewhere))
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testAWrongCodeStaysOnTheCodeStepWithAnInlineError() async {
        let linking = FakeAccountLinking(linkPhoneError: .invalidCode)
        let transport = StubTransport(scripted: [])
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "000000"
        let result = await model.link()

        XCTAssertEqual(result, .stay)
        XCTAssertEqual(model.step, .code(sentTo: "+15555550100"))
        XCTAssertEqual(model.caption, .error(LinkCopy.invalidCode))
        XCTAssertFalse(model.linkedElsewhere)
    }

    func testAPhoneTheBackendCannotFindAfterEveryWaitShowsTheServerMessageThenATapTriesAFreshKey() async throws {
        let problem = Self.problem(code: .phoneNotLinked, message: "Link a phone first.")
        let refusal = try StubTransport.Reply.problem(problem)
        let transport = StubTransport(scripted: [
            refusal, refusal, refusal, refusal, .json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)),
        ])
        let clock = TestClock()
        let linking = FakeAccountLinking()
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: clock)
        await model.sendCode(to: "+15555550100")
        model.code = "123456"

        let result = await Self.drive(clock, waits: LinkCopy.freshLinkWaits) { await model.link() }
        XCTAssertEqual(result, .toast("Link a phone first."))
        XCTAssertEqual(clock.state.current.requested, LinkCopy.freshLinkWaits)
        let retried = await model.link()

        XCTAssertNotNil(retried.finishedProfile)
        let calls = await linking.calls
        XCTAssertEqual(calls, [.sendPhoneCode("+15555550100"), .linkPhone("123456")])
        let keyHeader = try Self.keyHeader()
        let keys = await transport.sent.map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 5)
        XCTAssertEqual(Set(keys).count, 5, "a refusal is final, so each try sends a new key")
    }

    func testAFreshPhoneLinkTheBackendSeesAMomentLaterFinishes() async throws {
        let problem = Self.problem(code: .phoneNotLinked, message: "Link a phone first.")
        let transport = StubTransport(scripted: [
            try .problem(problem), .json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)),
        ])
        let clock = TestClock()
        let model = PhoneLinkModel(linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: clock)
        await model.sendCode(to: "+15555550100")
        model.code = "123456"

        let result = await Self.drive(clock, waits: [LinkCopy.freshLinkWaits[0]]) { await model.link() }

        XCTAssertEqual(result.finishedProfile?.authState, .awaitingSocials)
        XCTAssertNil(model.caption)
    }

    func testAFreshXLinkTheBackendSeesAMomentLaterFinishes() async throws {
        let problem = Self.problem(code: .xNotLinked, message: "Link X first.")
        let transport = StubTransport(scripted: [
            try .problem(problem),
            .json(.ok, Self.me(authState: "ONBOARDING_COMPLETED", phoneLinked: true, xUsername: "qa_x")),
        ])
        let clock = TestClock()
        let model = XLinkModel(linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: clock)

        let result = await Self.drive(clock, waits: [LinkCopy.freshLinkWaits[0]]) { await model.connect() }

        XCTAssertEqual(result.finishedProfile?.xUsername, "qa_x")
    }

    func testAStoreThatFailsOfflineRetriesWithoutSpendingTheCodeAgain() async throws {
        let linking = FakeAccountLinking()
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)),
        ])
        let model = PhoneLinkModel(linking: linking, onboarding: Self.onboarding(transport), clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "123456"
        let failed = await model.link()
        XCTAssertEqual(failed, .stay)
        XCTAssertEqual(model.caption, .error("You're offline. Try again."))
        let retried = await model.link()

        XCTAssertNotNil(retried.finishedProfile)
        let calls = await linking.calls
        XCTAssertEqual(calls, [.sendPhoneCode("+15555550100"), .linkPhone("123456")])
        let keyHeader = try Self.keyHeader()
        let keys = await transport.sent.map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testResendSendsToTheSameNumberAndClearsTheCode() async {
        let linking = FakeAccountLinking()
        let clock = TestClock()
        let model = PhoneLinkModel(
            linking: linking, onboarding: Self.onboarding(StubTransport(scripted: [])), clock: clock)

        await model.sendCode(to: "+15555550100")
        model.code = "12"
        await model.resend()
        XCTAssertEqual(model.code, "12", "a resend inside the cooldown does nothing")

        clock.advance(by: ResendCooldown.period)
        await model.resend()

        XCTAssertEqual(model.code, "")
        XCTAssertEqual(model.caption, .note(LinkCopy.newCodeSent))
        XCTAssertFalse(model.cooldown.canResend)
        let calls = await linking.calls
        XCTAssertEqual(calls, [.sendPhoneCode("+15555550100"), .sendPhoneCode("+15555550100")])
    }

    func testChangeNumberReturnsToNumberEntryWithTheCodeCleared() async {
        let model = PhoneLinkModel(
            linking: FakeAccountLinking(), onboarding: Self.onboarding(StubTransport(scripted: [])),
            clock: TestClock())

        await model.sendCode(to: "+15555550100")
        model.code = "123"
        model.changeNumber()

        XCTAssertEqual(model.step, .number)
        XCTAssertEqual(model.code, "")
    }

    func testSkippingPhoneThenLinkingXEndsWithTheReturnedProfile() async throws {
        let linking = FakeAccountLinking()
        let transport = StubTransport(scripted: [
            .json(.ok, Self.me(authState: "AWAITING_PHONE", phoneLinked: false)),
            .json(.ok, Self.me(authState: "AWAITING_PHONE", phoneLinked: false, xUsername: "qa_x")),
        ])
        let onboarding = Self.onboarding(transport)
        let phone = PhoneLinkModel(linking: linking, onboarding: onboarding, clock: TestClock())
        let x = XLinkModel(linking: linking, onboarding: onboarding, clock: TestClock())

        let skipResult = await phone.skip()
        let skipped = try XCTUnwrap(skipResult.finishedProfile)
        var cursor = OnboardingCursor.start.advanced(past: .phone)
        XCTAssertEqual(FirstRunGate.destination(for: skipped, onboardingCursor: cursor), .socials)
        let connectResult = await x.connect()
        let linked = try XCTUnwrap(connectResult.finishedProfile)
        cursor = cursor.advanced(past: .socials)

        XCTAssertEqual(linked.authState, .awaitingPhone)
        XCTAssertEqual(linked.xUsername, "qa_x")
        XCTAssertEqual(FirstRunGate.destination(for: linked, onboardingCursor: cursor), .app(restricted: false))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/onboarding/skip", "/v1/me/onboarding/socials"])
        let bodies = await transport.sentBodies
        let skipBody = try XCTUnwrap(bodies.first.flatMap { $0 })
        XCTAssertEqual(try JSONSerialization.jsonObject(with: skipBody) as? [String: String], ["step": "phone"])
        let calls = await linking.calls
        XCTAssertEqual(calls, [.linkX])
    }

    func testCancellingTheXSheetMakesNoRequestAndSaysNothing() async {
        let transport = StubTransport(scripted: [])
        let model = XLinkModel(
            linking: FakeAccountLinking(linkXError: .cancelled), onboarding: Self.onboarding(transport),
            clock: TestClock())

        let result = await model.connect()

        XCTAssertEqual(result, .stay)
        XCTAssertNil(model.caption)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testAnXAccountLinkedElsewhereSaysSo() async {
        let model = XLinkModel(
            linking: FakeAccountLinking(linkXError: .alreadyLinkedElsewhere),
            onboarding: Self.onboarding(StubTransport(scripted: [])), clock: TestClock())

        _ = await model.connect()

        XCTAssertTrue(model.linkedElsewhere)
        XCTAssertEqual(model.caption, .error(LinkCopy.xLinkedElsewhere))
    }

    func testSkippingXSendsTheSocialsStep() async throws {
        let transport = StubTransport(.json(.ok, Self.me(authState: "AWAITING_SOCIALS", phoneLinked: true)))
        let model = XLinkModel(
            linking: FakeAccountLinking(), onboarding: Self.onboarding(transport), clock: TestClock())

        let result = await model.skip()
        XCTAssertNotNil(result.finishedProfile)

        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["step": "socials"])
    }

    func testLinkCopyStaysClean() {
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(LinkCopy.auditedStrings))
    }

    func testTheLogRedactionHidesPhoneNumbers() {
        let line = LogRedaction.phoneNumbers(
            in: "apiError(400, Phone +1 (555) 555-0100 is linked; also 15555550100 and +447700900123)")

        XCTAssertFalse(line.contains("555"))
        XCTAssertFalse(line.contains("7700"))
        XCTAssertEqual(line.components(separatedBy: LogRedaction.phoneMarker).count - 1, 3)
        XCTAssertTrue(line.hasPrefix("apiError(400, Phone "), "short numbers such as a status stay")
    }

    private static func drive(
        _ clock: TestClock, waits: [Duration], _ body: @escaping @MainActor () async -> LinkStepResult
    ) async -> LinkStepResult {
        let task = Task { await body() }
        for wait in waits {
            _ = await clock.state.until { $0.pending == 1 }
            clock.advance(by: wait)
        }
        return await task.value
    }

    static func onboarding(_ transport: StubTransport) -> OnboardingAPI {
        OnboardingAPI(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
    }

    private static func keyHeader() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    static func problem(code: Components.Schemas.ErrorCode, message: String) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Rejected", status: 409, code: code, message: message,
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }

    static func me(authState: String, phoneLinked: Bool, xUsername: String? = nil) -> String {
        let x = xUsername.map { #""\#($0)""# } ?? "null"
        return """
            {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"qa_handle_1","display_name":"QA",\
            "photo_url":null,"auth_state":"\(authState)","account_status":"active","login_provider":"sms",\
            "member_wallet_address":"wallet-1","phone_linked":\(phoneLinked),"x_username":\(x),\
            "handle_changeable_at":null,"created_at":"2026-09-30T12:00:00Z"}
            """
    }
}

extension LinkStepResult {
    var finishedProfile: SessionProfile? {
        if case .finished(let profile) = self { profile } else { nil }
    }
}

actor FakeAccountLinking: AccountLinking {
    enum Call: Equatable {
        case sendPhoneCode(String)
        case linkPhone(String)
        case linkX
    }

    private(set) var calls: [Call] = []
    private let linkPhoneError: LinkError?
    private let linkXError: LinkError?

    init(linkPhoneError: LinkError? = nil, linkXError: LinkError? = nil) {
        self.linkPhoneError = linkPhoneError
        self.linkXError = linkXError
    }

    func sendPhoneCode(e164: String) async throws {
        calls.append(.sendPhoneCode(e164))
    }

    func linkPhone(code: String) async throws {
        calls.append(.linkPhone(code))
        if let linkPhoneError { throw linkPhoneError }
    }

    func linkX() async throws {
        calls.append(.linkX)
        if let linkXError { throw linkXError }
    }
}
