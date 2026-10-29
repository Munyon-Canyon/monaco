import Foundation
import HTTPTypes
import MonacoAPI
import MonacoNotify
import MonacoTestSupport
import XCTest

@MainActor
final class CatchUpModelTests: XCTestCase {
    private nonisolated static let now = Components.Schemas.CatchUp.sampleNow

    private static func body() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(Components.Schemas.CatchUp.six), as: UTF8.self)
    }

    private func make(
        _ replies: [StubTransport.Reply]
    ) -> (CatchUpModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: replies)
        let hints = FakeHintStream()
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        let model = CatchUpModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: { Self.now }, calendar: calendar)
        return (model, transport, hints)
    }

    func testLoadReadsOnceWithTheStartOfTheLocalDay() async throws {
        let (model, transport, _) = make([.json(.ok, try Self.body())])

        await model.load()

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen.count, 6)
        XCTAssertFalse(screen.isCaughtUp)
        XCTAssertEqual(screen.waiting.count, 1)
        XCTAssertEqual(screen.cabals.count, 3)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertTrue(sent.first?.path?.hasPrefix("/v1/me/catch-up?since=") ?? false)
    }

    func testAFailedFirstLoadFailsAndAFailedRefreshKeepsTheScreen() async throws {
        let (model, _, _) = make([
            .failure(URLError(.notConnectedToInternet)), .json(.ok, try Self.body()),
            .failure(URLError(.notConnectedToInternet)),
        ])

        await model.load()
        guard case .failed = model.state else { return XCTFail("not failed") }
        await model.load()
        await model.load()
        guard case .loaded = model.state else { return XCTFail("lost the screen") }
    }

    func testAHintWhileVisibleReloadsOnce() async throws {
        let (model, transport, hints) = make([.json(.ok, try Self.body()), .json(.ok, try Self.body())])
        await model.load()
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(1)
        model.setVisible(true)
        await hints.send(.changed(.user("me"), what: "catch_up_changed", id: "1"))
        for _ in 0..<500 where await transport.sent.count < 2 { await Task.yield() }
        observer.cancel()

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAnEmptyCatchUpIsCaughtUp() {
        let empty = Components.Schemas.CatchUp(
            count: 0, cabals: [], waiting: [], replies: [], invites: [], results: [], cleared: [])
        XCTAssertTrue(CatchUpScreen(empty).isCaughtUp)
    }

    private static let tslaKey = "proposal:00000000-0000-7000-8000-00000000b005"
    private static let nvdaKey = "proposal:00000000-0000-7000-8000-00000000b001"

    private static func encoded(_ key: String) -> String {
        key.replacingOccurrences(of: ":", with: "%3A")
    }

    private func header(_ request: HTTPRequest?) -> String? {
        guard let name = HTTPField.Name("Idempotency-Key") else { return nil }
        return request?.headerFields[name]
    }

    func testClearMovesTheRowAtOnceAndSendsOneKeyedPost() async throws {
        let (model, transport, _) = make([.json(.ok, try Self.body()), .gate])
        await model.load()

        let clearing = Task { await model.clear(Self.tslaKey) }
        await transport.waitForRequests(2)

        guard case .loaded(let during) = model.state else { return XCTFail("not loaded") }
        XCTAssertNil(during.item(key: Self.tslaKey))
        XCTAssertEqual(during.cleared.first?.key, Self.tslaKey)
        XCTAssertEqual(during.cleared.first?.clearedReason, .cleared)
        XCTAssertEqual(during.count, 5)
        await transport.releaseGate(.json(.noContent, ""))
        let cleared = await clearing.value

        XCTAssertTrue(cleared)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent.last?.method, .post)
        XCTAssertEqual(sent.last?.path, "/v1/me/catch-up/items/\(Self.encoded(Self.tslaKey))/clear")
        XCTAssertFalse(header(sent.last)?.isEmpty ?? true)
    }

    func testAFailedClearPutsTheRowBack() async throws {
        let (model, _, _) = make([.json(.ok, try Self.body()), .failure(URLError(.notConnectedToInternet))])
        await model.load()
        guard case .loaded(let before) = model.state else { return XCTFail("not loaded") }

        let cleared = await model.clear(Self.tslaKey)

        XCTAssertFalse(cleared)
        XCTAssertEqual(model.state, .loaded(before))
    }

    func testReadPutsOutTheDotAndAReadRowSendsNothing() async throws {
        let (model, transport, _) = make([.json(.ok, try Self.body()), .json(.noContent, "")])
        await model.load()

        await model.read(Self.nvdaKey)
        await model.read(Self.nvdaKey)

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen.item(key: Self.nvdaKey)?.unread, false)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent.last?.path, "/v1/me/catch-up/items/\(Self.encoded(Self.nvdaKey))/read")
    }

    func testAFailedReadBringsTheDotBack() async throws {
        let (model, _, _) = make([.json(.ok, try Self.body()), .failure(URLError(.notConnectedToInternet))])
        await model.load()

        await model.read(Self.nvdaKey)

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen.item(key: Self.nvdaKey)?.unread, true)
    }

    func testKeepPutsOutTheDotAndKeepsTheRow() async throws {
        let (model, transport, _) = make([.json(.ok, try Self.body()), .json(.noContent, "")])
        await model.load()

        await model.keep(Self.nvdaKey)

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen.item(key: Self.nvdaKey)?.unread, false)
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/me/catch-up/items/\(Self.encoded(Self.nvdaKey))/keep")
    }

    func testAHintDuringAWriteReloadsOnceAfterItSettles() async throws {
        let (model, transport, _) = make([
            .json(.ok, try Self.body()), .gate, .json(.ok, try Self.body()),
        ])
        await model.load()

        let clearing = Task { await model.clear(Self.tslaKey) }
        await transport.waitForRequests(2)
        await model.load()
        var sent = await transport.sent
        XCTAssertEqual(sent.count, 2, "no reload while the write is out")
        await transport.releaseGate(.json(.noContent, ""))
        _ = await clearing.value

        sent = await transport.sent
        XCTAssertEqual(sent.count, 3)
        XCTAssertTrue(sent.last?.path?.hasPrefix("/v1/me/catch-up?since=") ?? false)
    }

    func testTheCaughtUpSampleListsSixClearedRows() {
        let screen = CatchUpScreen(.caughtUp)
        XCTAssertTrue(screen.isCaughtUp)
        XCTAssertEqual(screen.cleared.count, 6)
        XCTAssertEqual(screen.cabals.count, 3)
        XCTAssertTrue(CatchUpScreen(.firstUse).cabals.isEmpty)
    }
}
