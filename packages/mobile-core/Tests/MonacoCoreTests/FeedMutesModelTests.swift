import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FeedMutesModelTests: XCTestCase {
    private let mutes = """
        [{"target_type":"item","target_id":"00000000-0000-7000-8000-0000000fe001","label":"Maya proposed","created_at":"2026-10-04T12:00:00Z"},\
        {"target_type":"kind","target_id":"trade","label":null,"created_at":"2026-10-03T12:00:00Z"}]
        """

    func testLoadListsTheMutesNewestFirstAndFallsBackToTheKindLabel() async {
        let model = makeModel(StubTransport(.json(.ok, mutes)))
        XCTAssertEqual(model.phase, .loading)
        await model.load()
        XCTAssertEqual(model.mutes.map(FeedMutesModel.label), ["Maya proposed", "Trades"])
    }

    func testNoMutesIsEmpty() async {
        let model = makeModel(StubTransport(.json(.ok, "[]")))
        await model.load()
        XCTAssertEqual(model.phase, .empty)
    }

    func testAFailedFirstLoadIsFailedAndRetrySucceeds() async {
        let model = makeModel(
            StubTransport(scripted: [.failure(URLError(.notConnectedToInternet)), .json(.ok, mutes)]))
        await model.load()
        guard case .failed = model.phase else { return XCTFail("\(model.phase)") }
        await model.load()
        XCTAssertEqual(model.mutes.count, 2)
    }

    func testUnmuteDeletesTheTargetAndDropsTheRowThenShowsEmptyAfterTheLast() async throws {
        let transport = StubTransport(scripted: [.json(.ok, mutes), .json(.noContent, ""), .json(.noContent, "")])
        let model = makeModel(transport)
        await model.load()
        let first = try XCTUnwrap(model.mutes.first)
        let message = await model.unmute(first)
        XCTAssertEqual(message, "Unmuted Maya proposed.")
        XCTAssertEqual(model.mutes.count, 1)
        let sent = await transport.sent
        XCTAssertEqual(sent[1].method, .delete)
        XCTAssertEqual(sent[1].path, "/v1/me/feed-mutes/item/00000000-0000-7000-8000-0000000fe001")
        let last = await model.unmute(try XCTUnwrap(model.mutes.first))
        XCTAssertEqual(last, "Unmuted Trades.")
        XCTAssertEqual(model.phase, .empty)
    }

    func testAFailedUnmuteKeepsTheRowAndBumpsTheTick() async throws {
        let model = makeModel(
            StubTransport(scripted: [.json(.ok, mutes), .failure(URLError(.notConnectedToInternet))]))
        await model.load()
        let message = await model.unmute(try XCTUnwrap(model.mutes.first))
        XCTAssertNil(message)
        XCTAssertEqual(model.mutes.count, 2)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    private func makeModel(_ transport: StubTransport) -> FeedMutesModel {
        FeedMutesModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport))
    }
}
