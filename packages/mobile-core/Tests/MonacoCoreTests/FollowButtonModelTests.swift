import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FollowButtonModelTests: XCTestCase {
    private static let userID = "00000000-0000-7000-8000-0000000000b1"

    func testFollowPostsFromTheProfileAndTakesTheAnswer() async throws {
        let transport = StubTransport(.json(.ok, #"{"following":true}"#))
        let model = makeModel(transport)
        await model.toggle()
        XCTAssertTrue(model.following)
        XCTAssertFalse(model.isToggling)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertEqual(sent.first?.path, "/v1/users/\(Self.userID)/follow")
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["source": "profile"])
    }

    func testUnfollowDeletesAndTakesTheAnswer() async {
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"following":true}"#),
            .json(.ok, #"{"following":false}"#),
        ])
        let model = makeModel(transport)
        await model.toggle()
        await model.toggle()
        XCTAssertFalse(model.following)
        let methods = await transport.sent.map(\.method)
        XCTAssertEqual(methods, [.post, .delete])
    }

    func testTheServerAnswerWinsOverTheOptimisticFlip() async {
        let transport = StubTransport(.json(.ok, #"{"following":false}"#))
        let model = makeModel(transport)
        await model.toggle()
        XCTAssertFalse(model.following)
    }

    func testAFailureRollsBackAndBumpsTheTick() async {
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"following":true}"#),
            Self.problem(500, "internal", "Something went wrong."),
        ])
        let model = makeModel(transport)
        await model.toggle()
        await model.toggle()
        XCTAssertTrue(model.following)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertNotNil(model.lastError)
        XCTAssertFalse(model.unavailable)
    }

    func testABannedUserIsUnavailableAndNotAFailure() async {
        let transport = StubTransport(Self.problem(403, "user_banned", "This account is banned."))
        let model = makeModel(transport)
        await model.toggle()
        XCTAssertTrue(model.unavailable)
        XCTAssertFalse(model.following)
        XCTAssertEqual(model.failureTick, 0)
        XCTAssertNil(model.lastError)
    }

    func testAnUnknownUserIsUnavailable() async {
        let transport = StubTransport(Self.problem(404, "user_not_found", "No such user."))
        let model = makeModel(transport)
        await model.toggle()
        XCTAssertTrue(model.unavailable)
    }

    func testEachTapSendsItsOwnIdempotencyKey() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"following":true}"#),
            .json(.ok, #"{"following":false}"#),
        ])
        let model = makeModel(transport)
        await model.toggle()
        await model.toggle()
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[name] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertNotNil(keys[1])
        XCTAssertNotEqual(keys[0], keys[1])
    }

    private func makeModel(_ transport: StubTransport) -> FollowButtonModel {
        FollowButtonModel(
            userID: Self.userID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}
