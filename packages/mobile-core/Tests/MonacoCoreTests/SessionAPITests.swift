import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class SessionAPITests: XCTestCase {
    func testTheProfileSampleHasTheExpectedMemberFields() {
        let sample = Components.Schemas.Me.sample

        XCTAssertEqual(sample.displayName, "Kai Cenat")
        XCTAssertEqual(sample.memberWalletAddress, "wallet-1")
    }

    func testTheDefaultSessionEndingHookDoesNothing() async throws {
        await DefaultSessionEndingTokens().endSession()
    }

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

    func testPatchMeSendsTheNameWithTheSessionBearer() async throws {
        let transport = StubTransport(.json(.ok, Self.meJSON))

        let profile = try await makeAPI(transport).patchMe(displayName: "New name")

        let sent = await transport.sent
        let idempotencyKey = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me"])
        XCTAssertEqual(sent.map(\.method), [.patch])
        XCTAssertEqual(sent.first?.headerFields[.authorization], "Bearer token-1")
        XCTAssertNotNil(sent.first?.headerFields[idempotencyKey])
        XCTAssertEqual(profile.displayName, "Kai Cenat")
    }

    func testUpdateDisplayNamePatchesTheNameAndReturnsTheServerProfile() async throws {
        let saved = Self.meJSON.replacingOccurrences(of: "Kai Cenat", with: "QA Name")
        let transport = StubTransport(.json(.ok, saved))

        let profile = try await makeAPI(transport).updateDisplayName("QA Name", submission: IdempotentSubmission())

        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me"])
        XCTAssertEqual(sent.map(\.method), [.patch])
        XCTAssertNotNil(sent.first?.headerFields[keyName])
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["display_name": "QA Name"])
        XCTAssertEqual(profile.displayName, "QA Name")
        XCTAssertEqual(profile.handle, "kaicenat")
    }

    func testARejectedDisplayNameMapsToTheInlineError() async throws {
        let rejected = Self.problem(.displayNameInvalid, status: 422, "Use letters and spaces.")
        let transport = try StubTransport.problem(rejected)

        do {
            _ = try await makeAPI(transport).updateDisplayName("x", submission: IdempotentSubmission())
            XCTFail("expected a problem")
        } catch let error as APIError {
            XCTAssertEqual(ProfileSaveFailure(error), .invalidName("Use letters and spaces."))
        }
    }

    func testARetriedNameAfterATransportErrorReusesTheKey() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, Self.meJSON),
        ])
        let api = makeAPI(transport)
        let submission = IdempotentSubmission()

        do {
            _ = try await api.updateDisplayName("Kai Cenat", submission: submission)
            XCTFail("expected the dropped connection")
        } catch let error as APIError {
            XCTAssertEqual(ProfileSaveFailure(error), .toast(ToastCopy.message(for: error)))
        }
        _ = try await api.updateDisplayName("Kai Cenat", submission: submission)

        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[keyName] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
    }

    func testUploadProfilePhotoSendsOneMultipartPhotoPart() async throws {
        let uploaded = Self.meJSON.replacingOccurrences(of: "kai.jpg", with: "new.jpg")
        let transport = StubTransport(.json(.ok, uploaded))
        let jpeg = Data([0xFF, 0xD8, 0xFF, 0xE0, 0x01, 0x02])

        let profile = try await makeAPI(transport).uploadProfilePhoto(jpeg, submission: IdempotentSubmission())

        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me/profile-photo"])
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertNotNil(sent.first?.headerFields[keyName])
        let contentType = try XCTUnwrap(sent.first?.headerFields[.contentType])
        XCTAssertTrue(contentType.hasPrefix("multipart/form-data; boundary="), contentType)
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        XCTAssertNotNil(body.range(of: Data(#"name="photo""#.utf8)))
        XCTAssertNotNil(body.range(of: jpeg))
        XCTAssertEqual(profile.photoURL?.absoluteString, "https://cdn.example.com/photos/new.jpg")
    }

    func testARateLimitedPhotoShowsTheServerMessageAndKeepsTheKey() async throws {
        let limited = try StubTransport.Reply.problem(
            Self.problem(.rateLimited, status: 429, "Slow down. Try again soon."))
        let transport = StubTransport(scripted: [limited, .json(.ok, Self.meJSON)])
        let api = makeAPI(transport)
        let submission = IdempotentSubmission()
        let photo = Data([0xFF, 0xD8, 0xFF])

        do {
            _ = try await api.uploadProfilePhoto(photo, submission: submission)
            XCTFail("expected rate_limited")
        } catch let error as APIError {
            XCTAssertEqual(ProfileSaveFailure(error), .toast("Slow down. Try again soon."))
        }
        XCTAssertTrue(submission.hasPendingKey, "a 429 is answered before the key is claimed")
        _ = try await api.uploadProfilePhoto(photo, submission: submission)

        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[keyName] }
        XCTAssertEqual(keys[0], keys[1])
        XCTAssertFalse(submission.hasPendingKey)
    }

    func testAFailureCarriesItsMessageWhereverItIsShown() {
        XCTAssertEqual(ProfileSaveFailure.invalidName("Use letters and spaces.").message, "Use letters and spaces.")
        XCTAssertEqual(ProfileSaveFailure.toast("Too many requests.").message, "Too many requests.")
        XCTAssertEqual(ProfileSaveFailure(.inFlight), .toast(ToastCopy.message(for: .inFlight)))
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

    func testARefreshed401ForMeDecodesTheRetried200() async throws {
        let retry = Self.meJSON.replacingOccurrences(of: "Kai Cenat", with: "Retried")
        let denied = StubTransport.Reply.response(
            status: .unauthorized, contentType: "application/problem+json",
            body: Data(#"{"status":401}"#.utf8)
        )
        let transport = StubTransport(scripted: [denied, .json(.ok, retry)])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh"])

        let profile = try await makeAPI(transport, tokens: tokens).me()

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me", "/v1/me"])
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

    private static func problem(
        _ code: Components.Schemas.ErrorCode, status: Int, _ message: String
    ) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank,
            title: "Rejected",
            status: status,
            code: code,
            message: message,
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
            retryable: false
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

private actor DefaultSessionEndingTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { nil }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
