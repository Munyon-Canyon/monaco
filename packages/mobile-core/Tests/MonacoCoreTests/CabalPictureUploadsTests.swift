import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class CabalPictureUploadsTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"

    func testSetPicturePutsTheBytesInThePictureFieldAndReturnsTheNewPicture() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(picture: #""https://cdn.test/cabal.jpg""#)))
        let image = Data([0xFF, 0xD8, 0xFF, 0xE0])

        let picture = try await uploads(transport).setPicture(
            cabalID: Self.cabalID, imageData: image, mimeType: "image/jpeg")

        XCTAssertEqual(picture, "https://cdn.test/cabal.jpg")
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.first)
        XCTAssertEqual(request.method, .put)
        XCTAssertEqual(request.path, "/v1/cabals/\(Self.cabalID)/picture")
        XCTAssertNotNil(request.headerFields[try idempotencyKey()])
        XCTAssertTrue(request.headerFields[.contentType]?.hasPrefix("multipart/form-data") == true)
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        let text = String(decoding: body, as: UTF8.self)
        XCTAssertTrue(text.contains(#"name="picture""#), text)
        XCTAssertTrue(text.contains(#"filename="cabal.jpg""#), text)
        XCTAssertNotNil(body.range(of: image))
    }

    func testRemovePictureDeletesAndReturnsNoPicture() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(picture: "null")))

        let picture = try await uploads(transport).removePicture(cabalID: Self.cabalID)

        XCTAssertNil(picture)
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.first)
        XCTAssertEqual(request.method, .delete)
        XCTAssertEqual(request.path, "/v1/cabals/\(Self.cabalID)/picture")
        XCTAssertNotNil(request.headerFields[try idempotencyKey()])
    }

    func testARetryAfterADroppedConnectionReusesTheKey() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, Self.cabal(picture: #""https://cdn.test/cabal.jpg""#)),
        ])
        let uploads = uploads(transport)
        let image = Data([0x01, 0x02])

        do {
            _ = try await uploads.setPicture(cabalID: Self.cabalID, imageData: image, mimeType: "image/jpeg")
            XCTFail("the first attempt should fail")
        } catch {}
        _ = try await uploads.setPicture(cabalID: Self.cabalID, imageData: image, mimeType: "image/jpeg")

        let key = try idempotencyKey()
        let keys = await transport.sent.map { $0.headerFields[key] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testStorageUnavailableCarriesTheServerMessage() async throws {
        let problem =
            #"{"type":"about:blank","title":"Service Unavailable","status":503,"code":"storage_unavailable","#
            + #""message":"Pictures can't be saved right now.","trace_id":"00000000000000000000000000000000","#
            + #""retryable":true}"#
        let transport = StubTransport(
            .response(status: .serviceUnavailable, contentType: "application/problem+json", body: Data(problem.utf8)))

        do {
            _ = try await uploads(transport).setPicture(
                cabalID: Self.cabalID, imageData: Data([1]), mimeType: "image/png")
            XCTFail("a 503 should throw")
        } catch let error as APIError {
            XCTAssertEqual(ToastCopy.message(for: error), "Pictures can't be saved right now.")
        }
    }

    private func idempotencyKey() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    private func uploads(_ transport: StubTransport) -> CabalPictureUploads {
        CabalPictureUploads(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private static func cabal(picture: String) -> String {
        ##"{"id":"\##(cabalID)","name":"QA pot","picture_url":\##(picture),"status":"active","##
            + ##""rules":{"join_mode":"open","voter_mode":"all","threshold":"majority","##
            + ##""proposal_expiry_seconds":86400,"slippage_bps":100},"##
            + ##""creator":{"user_id":"\##(cabalID)","handle":"kai","display_name":"Kai","photo_url":null},"##
            + ##""member_count":1,"members":[],"me":{"role":"creator","can_vote":true},"my_access_request":null,"##
            + ##""invite_code":"ABCD2345","treasury_address":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"}"##
    }
}
