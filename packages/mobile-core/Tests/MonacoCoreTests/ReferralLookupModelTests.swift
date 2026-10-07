import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ReferralLookupModelTests: XCTestCase {
    private let referrer =
        #"{"referrer":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","display_name":"Kai","#
        + #""photo_url":null,"handle":"kaicenat"}}"#

    func testAKnownCodeLoadsTheReferrer() async throws {
        let (model, transport) = try make(Self.lookup(referrer))

        await model.load()

        XCTAssertEqual(
            model.state,
            .loaded(.referrer(ReferralReferrer(userID: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "kaicenat"))))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/referrals/k7m4qx2p"])
    }

    func testAnUnknownCodeLoadsAsUnknown() async throws {
        let (model, _) = try make(Self.lookup(#"{"referrer":null}"#))

        await model.load()

        XCTAssertEqual(model.state, .loaded(.unknown))
    }

    func testATransportErrorFailsAndTheNextLoadRecovers() async throws {
        let (model, _) = try make(.failure(URLError(.notConnectedToInternet)), Self.lookup(referrer))

        await model.load()

        guard case .failed(.transport) = model.state else {
            return XCTFail("want a transport failure, got \(model.state)")
        }

        await model.load()

        guard case .loaded(.referrer) = model.state else {
            return XCTFail("want a referrer, got \(model.state)")
        }
    }

    private static func lookup(_ body: String) -> StubTransport.Reply {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        response.headerFields[.cacheControl] = "public, max-age=300"
        return .response(response, Data(body.utf8))
    }

    private func make(_ replies: StubTransport.Reply...) throws -> (ReferralLookupModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
        return (ReferralLookupModel(api: api, code: code), transport)
    }
}
