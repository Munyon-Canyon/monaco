import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class SessionAPITests: XCTestCase {
    func testOpenSessionSendsNoIdempotencyKey() async throws {
        let transport = StubTransport(.json(.ok, Self.meJSON))

        let profile = try await makeAPI(transport).openSession()

        let sent = await transport.sent
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/auth/session"])
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertNil(sent.first?.headerFields[name])
        XCTAssertEqual(profile.displayName, "Kai Cenat")
        XCTAssertEqual(profile.authState, .onboardingCompleted)
    }

    func testMeReadsWithoutAnIdempotencyKey() async throws {
        let transport = StubTransport(.json(.ok, Self.meJSON))
        let api = makeAPI(transport)

        let profile = try await api.me()

        let sent = await transport.sent
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me"])
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertNil(sent.first?.headerFields[name])
        XCTAssertEqual(profile.handle, "kaicenat")
        XCTAssertEqual(sent.first?.headerFields[.authorization], "Bearer token-1")
    }

    func testFractionalCreatedAtDecodesThroughTheClient() async throws {
        let json = Self.meJSON.replacingOccurrences(of: "2026-09-30T12:00:00Z", with: "2026-09-30T12:00:00.123456Z")
        let transport = StubTransport(.json(.ok, json))
        let whole = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-09-30T12:00:00Z"))

        let profile = try await makeAPI(transport).me()

        XCTAssertEqual(profile.createdAt.timeIntervalSince(whole), 0.123456, accuracy: 0.0000005)
    }

    func testUnknownStatesSurviveTheSessionCalls() async throws {
        let json = Self.meJSON
            .replacingOccurrences(of: "ONBOARDING_COMPLETED", with: "AWAITING_VIDEO")
            .replacingOccurrences(of: "\"account_status\":\"active\"", with: "\"account_status\":\"frozen\"")
        let transport = StubTransport(scripted: [
            .json(.ok, json),
            .json(.ok, json),
        ])
        let api = makeAPI(transport)

        let opened = try await api.openSession()
        let loaded = try await api.me()

        XCTAssertEqual(opened.authState, .unknown("AWAITING_VIDEO"))
        XCTAssertEqual(opened.accountStatus, .unknown("frozen"))
        XCTAssertEqual(opened.userID, "01890a5d-ac96-774b-bcce-b302099a8058")
        XCTAssertEqual(opened.memberWalletAddress, "wallet-1")
        XCTAssertEqual(loaded.authState, opened.authState)
        XCTAssertEqual(loaded.accountStatus, opened.accountStatus)
    }

    func testABadCreatedAtFromTheClientIsDecoding() async throws {
        let json = Self.meJSON.replacingOccurrences(of: "2026-09-30T12:00:00Z", with: "yesterday")
        let transport = StubTransport(.json(.ok, json))

        do {
            _ = try await makeAPI(transport).me()
            XCTFail("expected decoding")
        } catch APIError.decoding {
        } catch {
            XCTFail("expected .decoding, got \(error)")
        }
    }

    func testAccountDeletedThrowsAccountDeleted() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank,
                title: "Gone",
                status: 403,
                code: .accountDeleted,
                message: "This account was deleted.",
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: false
            ))

        do {
            _ = try await makeAPI(transport).openSession()
            XCTFail("expected accountDeleted")
        } catch APIError.accountDeleted {
        } catch {
            XCTFail("expected .accountDeleted, got \(error)")
        }
    }

    func testARefreshed401InsideSessionBodyDecodesTheRetried200() async throws {
        let retry = Self.meJSON.replacingOccurrences(of: "Kai Cenat", with: "Retried")
        let denied = StubTransport.Reply.response(
            status: .unauthorized, contentType: "application/problem+json",
            body: Data(#"{"status":401}"#.utf8)
        )
        let transport = StubTransport(scripted: [denied, .json(.ok, retry)])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh"])

        let profile = try await makeAPI(transport, tokens: tokens).openSession()

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/auth/session", "/v1/auth/session"])
        XCTAssertEqual(sent.map { $0.headerFields[.authorization] }, ["Bearer stale", "Bearer fresh"])
        XCTAssertEqual(profile.displayName, "Retried")
    }

    func testASessionBodyOver64KiBSurfacesAsAnError() async throws {
        let body = Data(repeating: 0x78, count: 64 * 1024 + 1)
        let transport = StubTransport(.response(status: .ok, contentType: "application/json", body: body))

        do {
            _ = try await makeAPI(transport).me()
            XCTFail("expected the oversized body to fail")
        } catch {
            if case APIError.decoding("session") = error {
                XCTFail("oversized body was reported as a missing session")
            }
        }
    }

    private func makeAPI(
        _ transport: StubTransport,
        tokens: StubTokenProvider = StubTokenProvider(token: "token-1")
    ) -> SessionAPI {
        SessionAPI(
            api: APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)
        )
    }

    private static let meJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kaicenat","display_name":"Kai Cenat",\
        "photo_url":"https://cdn.example.com/photos/kai.jpg","auth_state":"ONBOARDING_COMPLETED",\
        "account_status":"active","member_wallet_address":"wallet-1","phone_linked":true,\
        "x_username":"kaicenat","handle_changeable_at":"2026-10-30T12:00:00Z",\
        "created_at":"2026-09-30T12:00:00Z"}
        """
}
