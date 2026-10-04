import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class PushRegistrationTests: XCTestCase {
    private static let token = String(repeating: "ab", count: 32)
    private static let rotated = String(repeating: "cd", count: 32)
    private static let noContent = StubTransport.Reply.response(
        status: .noContent, contentType: "application/json", body: Data())

    func testTheHexOfAKnownTokenIsLowercaseWithTwoDigitsPerByte() {
        let block: [UInt8] = [0x00, 0x01, 0x0A, 0x0F, 0x10, 0xAB, 0xCD, 0xFF]

        let hex = PushDeviceToken.hex(Data(Array(repeating: block, count: 4).flatMap { $0 }))

        XCTAssertEqual(hex, String(repeating: "00010a0f10abcdff", count: 4))
    }

    func testTheEnvironmentComesFromTheInfoValueOrIsUnknown() {
        XCTAssertEqual(PushEnvironment(infoValue: "sandbox"), .sandbox)
        XCTAssertEqual(PushEnvironment(infoValue: "production"), .production)
        for unknown in [nil, "", "staging", "Sandbox", "$(MONACO_APS_ENVIRONMENT)"] {
            XCTAssertNil(PushEnvironment(infoValue: unknown), "\(unknown ?? "nil")")
        }
    }

    func testOnlyASignedInAuthorizedMemberRegisters() {
        let registers: [PushAuthorizationStatus: Bool] = [
            .notDetermined: false,
            .denied: false,
            .authorized: true,
            .provisional: false,
            .ephemeral: false,
        ]

        XCTAssertEqual(Set(registers.keys), Set(PushAuthorizationStatus.allCases))
        for (status, expected) in registers {
            XCTAssertEqual(
                PushRegistrationPolicy.shouldRegister(status: status, isSignedIn: true), expected, "\(status)")
            XCTAssertFalse(
                PushRegistrationPolicy.shouldRegister(status: status, isSignedIn: false), "\(status) signed out")
        }
    }

    func testAnIdenticalRegistrationIsPostedOncePerLaunch() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)

        try await registrar.register(token: Self.token)
        try await registrar.register(token: Self.token)

        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices"])
    }

    func testARotatedTokenIsPosted() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)

        try await registrar.register(token: Self.token)
        try await registrar.register(token: Self.rotated)

        let requests = await requests(transport)
        let tokens = try await postedTokens(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "POST /v1/devices"])
        XCTAssertEqual(tokens, [Self.token, Self.rotated])
    }

    func testAFailedRegistrationIsPostedAgainByTheNextCallAndThenNotAgain() async throws {
        let offline = URLError(.notConnectedToInternet)
        let transport = StubTransport(scripted: [.failure(offline), Self.noContent])
        let registrar = makeRegistrar(transport)

        do {
            try await registrar.register(token: Self.token)
            XCTFail("expected the dropped connection")
        } catch {
            XCTAssertEqual(error as? APIError, .transport(offline))
        }
        try await registrar.register(token: Self.token)
        try await registrar.register(token: Self.token)

        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "POST /v1/devices"])
    }

    func testUnregisterDeletesTheLastRegisteredToken() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)
        try await registrar.register(token: Self.token)
        try await registrar.register(token: Self.rotated)

        try await registrar.unregister(within: .seconds(3))

        let requests = await requests(transport)
        XCTAssertEqual(
            requests, ["POST /v1/devices", "POST /v1/devices", "DELETE /v1/devices/\(Self.rotated)"])
    }

    func testUnregisterWithNoTokenSendsNothing() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)

        try await registrar.unregister(within: .seconds(3))

        let requests = await requests(transport)
        XCTAssertEqual(requests, [])
    }

    func testUnregisterLetsTheNextMemberRegisterTheSameToken() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)
        try await registrar.register(token: Self.token)
        try await registrar.unregister(within: .seconds(3))

        try await registrar.register(token: Self.token)

        let requests = await requests(transport)
        XCTAssertEqual(
            requests, ["POST /v1/devices", "DELETE /v1/devices/\(Self.token)", "POST /v1/devices"])
    }

    func testResetDeletesNothingAndLetsTheSameTokenRegisterAgain() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport)
        try await registrar.register(token: Self.token)

        await registrar.reset()
        try await registrar.unregister(within: .seconds(3))
        try await registrar.register(token: Self.token)

        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "POST /v1/devices"])
    }

    func testARegistrationThatFinishesAfterAResetIsNotRemembered() async throws {
        let transport = StubTransport(scripted: [.gate, Self.noContent])
        let registrar = makeRegistrar(transport)
        let token = Self.token
        let posting = Task { try await registrar.register(token: token) }
        await transport.waitForRequest()

        await registrar.reset()
        await transport.releaseGate(Self.noContent)
        try await posting.value
        try await registrar.register(token: token)

        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "POST /v1/devices"])
    }

    func testAnUnknownEnvironmentPostsNothing() async throws {
        let transport = StubTransport(Self.noContent)
        let registrar = makeRegistrar(transport, environment: nil)

        do {
            try await registrar.register(token: Self.token)
            XCTFail("expected the unknown environment")
        } catch {
            XCTAssertEqual(error as? PushRegistrationError, .unknownEnvironment)
        }

        let requests = await requests(transport)
        XCTAssertEqual(requests, [])
    }

    func testAFailedUnregisterThrowsAndTheTokenIsNotDeletedTwice() async throws {
        let timedOut = URLError(.timedOut)
        let transport = StubTransport(scripted: [Self.noContent, .failure(timedOut)])
        let registrar = makeRegistrar(transport)
        try await registrar.register(token: Self.token)

        do {
            try await registrar.unregister(within: .seconds(3))
            XCTFail("expected the failed delete")
        } catch {
            XCTAssertEqual(error as? APIError, .transport(timedOut))
        }
        try await registrar.unregister(within: .seconds(3))

        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "DELETE /v1/devices/\(Self.token)"])
    }

    func testUnregisterGivesUpAtTheLimitAndCancelsTheDelete() async throws {
        let transport = StubTransport(scripted: [Self.noContent, .hang])
        let clock = TestClock()
        let registrar = makeRegistrar(transport, clock: clock)
        try await registrar.register(token: Self.token)

        let unregistering = Task { try await registrar.unregister(within: .seconds(3)) }
        _ = await clock.state.until { $0.pending == 1 }
        XCTAssertEqual(clock.state.current.requested, [.seconds(3)])
        clock.advance(by: .seconds(3))
        let outcome = await unregistering.result

        XCTAssertThrowsError(try outcome.get()) { error in
            XCTAssertEqual(error as? URLError, URLError(.timedOut))
        }
        let requests = await requests(transport)
        XCTAssertEqual(requests, ["POST /v1/devices", "DELETE /v1/devices/\(Self.token)"])
    }

    func testRegisterPostsTheTokenAndEnvironmentWithAKey() async throws {
        let transport = StubTransport(Self.noContent)

        try await makeAPI(transport).register(token: Self.token, environment: .production)

        let all = await transport.sent
        let sent = try XCTUnwrap(all.first)
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.method, .post)
        XCTAssertEqual(sent.path, "/v1/devices")
        XCTAssertEqual(sent.headerFields[.authorization], "Bearer token-1")
        XCTAssertNotNil(sent.headerFields[keyName])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["token": Self.token, "environment": "production"])
    }

    func testUnregisterDeletesTheTokenWithAKey() async throws {
        let transport = StubTransport(Self.noContent)

        try await makeAPI(transport).unregister(token: Self.token)

        let all = await transport.sent
        let sent = try XCTUnwrap(all.first)
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.method, .delete)
        XCTAssertEqual(sent.path, "/v1/devices/\(Self.token)")
        XCTAssertEqual(sent.headerFields[.authorization], "Bearer token-1")
        XCTAssertNotNil(sent.headerFields[keyName])
        let bodies = await transport.sentBodies
        XCTAssertEqual(bodies, [nil])
    }

    private func makeAPI(_ transport: StubTransport) -> DeviceAPI {
        DeviceAPI(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private func makeRegistrar(
        _ transport: StubTransport,
        environment: PushEnvironment? = .sandbox,
        clock: TestClock = TestClock()
    ) -> PushRegistrar {
        PushRegistrar(service: makeAPI(transport), environment: environment, clock: clock)
    }

    private func requests(_ transport: StubTransport) async -> [String] {
        let sent = await transport.sent
        return sent.map { "\($0.method.rawValue) \($0.path ?? "")" }
    }

    private func postedTokens(_ transport: StubTransport) async throws -> [String] {
        let bodies = await transport.sentBodies
        return try bodies.map { body in
            let data = try XCTUnwrap(body)
            let json = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: String])
            return try XCTUnwrap(json["token"])
        }
    }
}
