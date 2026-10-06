import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedMuteModelTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testMutingACabalSendsOneKeyedPutAndDropsItsRows() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .json(.noContent, "")])
        let model = makeModel(transport)
        await model.load()
        let option = try XCTUnwrap(model.muteOptions(for: samples[0]).first { $0.menuTitle.hasPrefix("Mute Weekend") })
        let receipt = await model.mute(option)
        XCTAssertEqual(receipt?.message, "Muted Weekend investors.")
        XCTAssertEqual(model.items.map(\.id), [samples[2].id])
        let sent = await transport.sent
        XCTAssertEqual(sent[1].method, .put)
        XCTAssertEqual(sent[1].path, "/v1/me/feed-mutes")
        XCTAssertNotNil(sent[1].headerFields.first { $0.name.canonicalName == "idempotency-key" })
        let body = await transport.sentBodies[1].flatMap { String(data: $0, encoding: .utf8) }
        XCTAssertEqual(
            try JSONSerialization.jsonObject(with: Data(try XCTUnwrap(body).utf8)) as? [String: String],
            ["target_type": "cabal", "target_id": Components.Schemas.FeedItem.sampleCabalID])
    }

    func testAFailedMuteKeepsTheRowsAndBumpsTheTick() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)), .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        let receipt = await model.mute(.hide(samples[0]))
        XCTAssertNil(receipt)
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testUndoDeletesTheMuteAndReloadsPageOneSoTheRowsReturn() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)), .json(.noContent, ""), .json(.noContent, ""),
            .json(.ok, try Self.page(samples)),
        ])
        let model = makeModel(transport)
        await model.load()
        let muted = await model.mute(.hide(samples[0]))
        let receipt = try XCTUnwrap(muted)
        XCTAssertEqual(model.items.count, samples.count - 1)
        await model.undo(receipt)
        XCTAssertEqual(model.items.map(\.id), samples.map(\.id))
        let sent = await transport.sent
        XCTAssertEqual(sent[2].method, .delete)
        XCTAssertEqual(sent[2].path, "/v1/me/feed-mutes/item/\(samples[0].id)")
    }

    func testAFailedUndoDoesNotReload() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)), .json(.noContent, ""), .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        let muted = await model.mute(.hide(samples[0]))
        let receipt = try XCTUnwrap(muted)
        await model.undo(receipt)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 3)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testMyCabalsSendsScopeMine() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(samples)))
        let model = makeModel(transport)
        await model.select(.mine)
        let path = await transport.sent.last?.path ?? ""
        XCTAssertTrue(path.contains("scope=mine"), path)
    }

    private func makeModel(_ transport: StubTransport) -> FeedModel {
        FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport),
            viewerID: nil, hints: FakeHintStream(), clock: TestClock())
    }

    private static func page(_ items: [Components.Schemas.FeedItem]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(
            decoding: try encoder.encode(Components.Schemas.FeedPage(items: items, nextCursor: nil)), as: UTF8.self)
    }
}
