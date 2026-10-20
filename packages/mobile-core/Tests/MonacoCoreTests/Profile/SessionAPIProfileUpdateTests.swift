import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class SessionAPIProfileUpdateTests: XCTestCase {
    func testUpdateProfileSendsTheBioAndTheSharePerformanceSetting() async throws {
        let saved = Self.meJSON.replacingOccurrences(
            of: #""created_at""#, with: #""bio":"Building in public.\nNYC","share_performance":false,"created_at""#)
        let transport = StubTransport(.json(.ok, saved))

        let profile = try await makeAPI(transport).updateProfile(
            ProfileUpdate(bio: "  Building in public.\nNYC \n", sharePerformance: false),
            submission: IdempotentSubmission())

        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me"])
        XCTAssertEqual(sent.map(\.method), [.patch])
        XCTAssertNotNil(sent.first?.headerFields[keyName])
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: Any])
        XCTAssertEqual(json["bio"] as? String, "Building in public.\nNYC")
        XCTAssertEqual(json["share_performance"] as? Bool, false)
        XCTAssertNil(json["display_name"])
        XCTAssertEqual(profile.bio, "Building in public.\nNYC")
        XCTAssertFalse(profile.sharePerformance)
    }

    func testClearingTheBioSendsAnEmptyStringAndLeavesTheSettingOut() async throws {
        let transport = StubTransport(.json(.ok, Self.meJSON))

        let profile = try await makeAPI(transport).updateProfile(
            ProfileUpdate(bio: " \n"), submission: IdempotentSubmission())

        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: Any])
        XCTAssertEqual(json["bio"] as? String, "")
        XCTAssertNil(json["share_performance"])
        XCTAssertNil(profile.bio)
        XCTAssertTrue(profile.sharePerformance)
    }

    private func makeAPI(_ transport: StubTransport) -> SessionAPI {
        SessionAPI(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private static let meJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kaicenat","display_name":"Kai Cenat",\
        "auth_state":"ONBOARDING_COMPLETED","account_status":"active","member_wallet_address":"wallet-1",\
        "phone_linked":true,"created_at":"2026-09-30T12:00:00Z"}
        """
}
