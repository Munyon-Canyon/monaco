import Foundation
import HTTPTypes
import OpenAPIRuntime
import XCTest

@testable import MonacoAPI

@MainActor
final class ChatMomentFilterTests: XCTestCase {
    private static let base = URL(string: "https://example.test")!
    private static let known = #"{"id":"a","kind":"system","moment":{"kind":"cabal_created"}}"#
    private static let unknown = #"{"id":"b","kind":"system","moment":{"kind":"something_new"}}"#
    private static let user = #"{"id":"c","kind":"user","body":"gm"}"#

    private func run(
        method: HTTPRequest.Method = .get, path: String = "/v1/cabals/c1/messages?moments=true",
        status: HTTPResponse.Status = .ok, body: String
    ) async throws -> String {
        let request = HTTPRequest(method: method, scheme: nil, authority: nil, path: path)
        let (_, out) = try await ChatMomentFilterMiddleware().intercept(
            request, body: nil, baseURL: Self.base, operationID: "op"
        ) { _, _, _ in
            (HTTPResponse(status: status), HTTPBody(body))
        }
        let collected = try await Data(collecting: XCTUnwrap(out), upTo: 1 << 20)
        return String(decoding: collected, as: UTF8.self)
    }

    private func ids(_ json: String, key: String = "messages") throws -> [String] {
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(json.utf8)) as? [String: Any])
        let rows = try XCTUnwrap(object[key] as? [[String: Any]])
        return rows.compactMap { $0["id"] as? String }
    }

    func testAnUnknownKindIsDroppedFromAPage() async throws {
        let out = try await run(body: #"{"messages":[\#(Self.known),\#(Self.unknown),\#(Self.user)]}"#)
        XCTAssertEqual(try ids(out), ["a", "c"])
    }

    func testKnownKindsAreKept() async throws {
        let body = #"{"messages":[\#(Self.known),\#(Self.user)]}"#
        let out = try await run(body: body)
        XCTAssertEqual(out, body)
    }

    func testASettledVoteMomentIsKept() async throws {
        let settled = #"{"id":"e","kind":"system","moment":{"kind":"proposal_settled"}}"#
        let body = #"{"messages":[\#(settled),\#(Self.user)]}"#
        let out = try await run(body: body)
        XCTAssertEqual(out, body)
    }

    func testAThreadReadDropsUnknownReplies() async throws {
        let body = #"{"parent":\#(Self.user),"replies":[\#(Self.unknown),\#(Self.user)]}"#
        let out = try await run(path: "/v1/cabals/c1/messages/m1/thread", body: body)
        XCTAssertEqual(try ids(out, key: "replies"), ["c"])
    }

    func testAMomentWithoutAKindIsDropped() async throws {
        let bare = #"{"id":"d","kind":"system","moment":{}}"#
        let out = try await run(body: #"{"messages":[\#(bare),\#(Self.user)]}"#)
        XCTAssertEqual(try ids(out), ["c"])
    }

    func testANonChatPathIsUntouched() async throws {
        let body = #"{"messages":[\#(Self.unknown)]}"#
        let out = try await run(path: "/v1/cabals/c1/members", body: body)
        XCTAssertEqual(out, body)
        let other = try await run(path: "/v1/cabals/c1/messages/m1/reactions/x", body: body)
        XCTAssertEqual(other, body)
        let missing = try await run(path: "/v1/me", body: body)
        XCTAssertEqual(missing, body)
    }

    func testAnUnparsableBodyIsUntouched() async throws {
        let out = try await run(body: "not json")
        XCTAssertEqual(out, "not json")
    }

    func testANonOkResponseIsUntouched() async throws {
        let body = #"{"messages":[\#(Self.unknown)]}"#
        let out = try await run(status: .badRequest, body: body)
        XCTAssertEqual(out, body)
    }

    func testAPostIsUntouched() async throws {
        let body = #"{"messages":[\#(Self.unknown)]}"#
        let out = try await run(method: .post, body: body)
        XCTAssertEqual(out, body)
    }

    func testAResponseWithoutABodyPassesThrough() async throws {
        let request = HTTPRequest(method: .get, scheme: nil, authority: nil, path: "/v1/cabals/c1/messages")
        let (_, out) = try await ChatMomentFilterMiddleware().intercept(
            request, body: nil, baseURL: Self.base, operationID: "op"
        ) { _, _, _ in
            (HTTPResponse(status: .ok), nil)
        }
        XCTAssertNil(out)
    }
}
