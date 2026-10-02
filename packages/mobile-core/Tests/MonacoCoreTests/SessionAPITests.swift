import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class SessionAPITests: XCTestCase {
    func testOpenSessionSendsAnIdempotencyKeyAndReusesItAfterATransportError() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.cannotConnectToHost)),
            .json(.ok, Self.meJSON),
        ])
        let submission = IdempotentSubmission { "key-1" }
        let api = makeAPI(transport, submission: submission)

        do {
            _ = try await api.openSession()
            XCTFail("expected the transport error")
        } catch let APIError.transport(error) {
            XCTAssertEqual(error.code, .cannotConnectToHost)
        }

        let profile = try await api.openSession()

        let sent = await transport.sent
        let posts = sent.filter { $0.path == "/v1/auth/session" }
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = posts.map { $0.headerFields[name] }
        XCTAssertEqual(posts.map(\.method), [.post, .post])
        XCTAssertEqual(keys, ["key-1", "key-1"])
        XCTAssertEqual(profile.displayName, "Kai Cenat")
        XCTAssertEqual(profile.authState, .onboardingCompleted)
        XCTAssertFalse(submission.hasPendingKey)
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
        let submission = IdempotentSubmission { "key-1" }
        let api = makeAPI(transport, submission: submission)

        let opened = try await api.openSession()
        let loaded = try await api.me()

        XCTAssertEqual(opened.authState, .unknown("AWAITING_VIDEO"))
        XCTAssertEqual(opened.accountStatus, .unknown("frozen"))
        XCTAssertEqual(opened.userID, "01890a5d-ac96-774b-bcce-b302099a8058")
        XCTAssertEqual(opened.memberWalletAddress, "wallet-1")
        XCTAssertEqual(loaded.authState, opened.authState)
        XCTAssertEqual(loaded.accountStatus, opened.accountStatus)
        XCTAssertFalse(submission.hasPendingKey)
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

    private func makeAPI(
        _ transport: StubTransport,
        submission: IdempotentSubmission = IdempotentSubmission()
    ) -> SessionAPI {
        SessionAPI(
            api: APIClient(
                serverURL: testServerURL,
                tokens: StubTokenProvider(token: "token-1"),
                transport: transport
            ),
            submission: submission
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
