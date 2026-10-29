import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalOrderModelTests: XCTestCase {
    private let ids = ["a", "b", "c"]

    func testADropPostsTheCompleteOrderOnceAndReloadsAfterTheNoContent() async throws {
        let transport = StubTransport(routes: ["/v1/me/cabals/order": [Self.noContent]])
        var reloads = 0
        let model = CabalOrderModel(api: api(transport), saved: { reloads += 1 })
        model.sync(ids)

        await model.move("c", before: "a")

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/cabals/order"])
        XCTAssertEqual(sent.map(\.method), [.put])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        let decoded = try JSONDecoder().decode([String: [String]].self, from: body)
        XCTAssertEqual(decoded["cabal_ids"], ["c", "a", "b"])
        XCTAssertEqual(model.order, ["c", "a", "b"])
        XCTAssertEqual(reloads, 1)
        XCTAssertEqual(model.failureTick, 0)
    }

    func testAFailedSaveRevertsTheOrderAndRaisesAToast() async throws {
        let transport = StubTransport(routes: [
            "/v1/me/cabals/order": [.json(.internalServerError, "{}")]
        ])
        var reloads = 0
        let model = CabalOrderModel(api: api(transport), saved: { reloads += 1 })
        model.sync(ids)

        await model.move("c", before: "a")

        XCTAssertEqual(model.order, ids)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertNotNil(model.lastError)
        XCTAssertEqual(reloads, 0)
    }

    func testMoveEarlierAndLaterSwapNeighbours() async throws {
        let transport = StubTransport(routes: [
            "/v1/me/cabals/order": [Self.noContent, Self.noContent]
        ])
        let model = CabalOrderModel(api: api(transport))
        model.sync(ids)

        await model.moveEarlier("b")
        XCTAssertEqual(model.order, ["b", "a", "c"])
        await model.moveLater("b")

        XCTAssertEqual(model.order, ["a", "b", "c"])
    }

    func testTheEdgesDoNotMove() async {
        let transport = StubTransport(routes: [:])
        let model = CabalOrderModel(api: api(transport))
        model.sync(ids)

        await model.moveEarlier("a")
        await model.moveLater("c")
        await model.move("a", before: "a")

        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
        XCTAssertEqual(model.order, ids)
    }

    private static let noContent: StubTransport.Reply = .response(
        status: .noContent, contentType: "application/json", body: Data())

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
