import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ActivityLookupTests: XCTestCase {
    func testANonMemberLooksUpAPublishedSwapThroughTheSwapEndpoint() async throws {
        let (model, transport) = make([Self.problem(403, "not_cabal_member"), Self.swap])

        let lookup = await model.lookup(id: "swap-1")

        guard case .found(let row) = lookup else { return XCTFail("want found, got \(lookup)") }
        XCTAssertEqual(row.title, "Bought Apple")
        XCTAssertEqual(row.status, .confirmed)
        XCTAssertNil(row.actorName)
        XCTAssertEqual(model.openSwap?.statusLabel, "Done")
        XCTAssertEqual(model.openSwap?.retryable, false)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/cabals/cabal-1/activity?limit=100", "/v1/swaps/swap-1"])
    }

    func testANonMemberRefusedTheSwapIsForbidden() async {
        let (model, _) = make([Self.problem(403, "not_cabal_member"), Self.problem(403, "not_cabal_member")])

        let lookup = await model.lookup(id: "swap-1")

        XCTAssertEqual(lookup, .forbidden)
    }

    func testAnUnknownSwapIsNotFound() async {
        let (model, _) = make([Self.problem(403, "not_cabal_member"), Self.problem(404, "swap_not_found")])

        let lookup = await model.lookup(id: "swap-1")

        XCTAssertEqual(lookup, .notFound)
    }

    func testATransientSwapFailureIsRetryable() async {
        let (model, _) = make([Self.problem(403, "not_cabal_member"), .failure(URLError(.timedOut))])

        let lookup = await model.lookup(id: "swap-1")

        XCTAssertEqual(lookup, .failed)
    }

    private static let swap = StubTransport.Reply.json(
        .ok,
        #"{"id":"swap-1","cabal_id":"cabal-2","source":{"kind":"proposal","id":"p-1"},"action":"buy","#
            + #""symbol":"AAPLx","asset_name":"Apple","token_decimals":8,"usdc_micros":25000000,"#
            + #""token_amount":null,"status":"confirmed","failure_code":null,"failure_message":null,"#
            + #""tx_signature":null,"created_at":"2026-10-03T15:00:00Z","confirmed_at":"2026-10-03T15:01:00Z","#
            + #""retryable":false}"#)

    private static func problem(_ status: Int, _ code: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"No.","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: HTTPResponse.Status(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }

    private func make(_ script: [StubTransport.Reply]) -> (CabalActivityModel, StubTransport) {
        let transport = StubTransport(scripted: script)
        let now = Date(timeIntervalSince1970: 1_791_100_800)
        let model = CabalActivityModel(
            cabalID: "cabal-1",
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream(), clock: { now })
        return (model, transport)
    }
}
