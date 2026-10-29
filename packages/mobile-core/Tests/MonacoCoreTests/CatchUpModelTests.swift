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

    private static let tsla = "proposal:00000000-0000-7000-8000-00000000b005"
    private static let nvda = "proposal:00000000-0000-7000-8000-00000000b001"

    private func loaded(
        _ replies: [StubTransport.Reply]
    ) async throws -> (CatchUpModel, StubTransport, FakeHintStream) {
        let made = make([.json(.ok, try Self.body())] + replies)
        await made.0.load()
        return made
    }

    private func screen(_ model: CatchUpModel) throws -> CatchUpScreen {
        guard case .loaded(let screen) = model.state else { throw XCTSkip("not loaded") }
        return screen
    }

    private func path(_ key: String, _ verb: String) -> String {
        "/v1/me/catch-up/items/\(key.replacingOccurrences(of: ":", with: "%3A"))/\(verb)"
    }

    private func noContent() -> StubTransport.Reply {
        .response(status: .noContent, contentType: "application/json", body: Data())
    }

    private func idempotencyKeys(_ transport: StubTransport) async throws -> [String?] {
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        return await transport.sent.filter { $0.method == .post }.map { $0.headerFields[name] }
    }

    func testClearMovesTheRowAtOnceAndPostsOnceWithAKey() async throws {
        let (model, transport, _) = try await loaded([.gate])

        let task = Task { await model.clear(Self.tsla) }
        await transport.waitForRequests(2)
        let during = try screen(model)
        XCTAssertFalse(during.results.contains { $0.key == Self.tsla })
        XCTAssertEqual(during.cleared.first?.key, Self.tsla)
        XCTAssertEqual(during.cleared.first?.clearedReason, .cleared)
        XCTAssertEqual(during.count, 5)
        await transport.releaseGate(noContent())
        let ok = await task.value

        XCTAssertTrue(ok)
        let sent = await transport.sent.filter { $0.method == .post }
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.path, path(Self.tsla, "clear"))
        let keys = try await idempotencyKeys(transport)
        XCTAssertNotNil(keys.first ?? nil)
    }

    func testAFailedClearPutsTheRowBackAtItsPlace() async throws {
        let (model, _, _) = try await loaded([.failure(URLError(.notConnectedToInternet))])
        let before = try screen(model)

        let ok = await model.clear(Self.tsla)

        XCTAssertFalse(ok)
        XCTAssertEqual(try screen(model), before)
    }

    func testClearingAMissingRowOrBeforeLoadSendsNothing() async throws {
        let (model, transport, _) = try await loaded([])
        let missing = await model.clear("proposal:nope")
        XCTAssertFalse(missing)

        let (idle, idleTransport, _) = make([])
        let early = await idle.clear(Self.tsla)
        XCTAssertFalse(early)
        let sentCount = await transport.sent.count
        let idleCount = await idleTransport.sent.count
        XCTAssertEqual(sentCount, 1)
        XCTAssertEqual(idleCount, 0)
    }

    func testReadPutsOutTheDotAndAFailureLightsItAgain() async throws {
        let (model, transport, _) = try await loaded([noContent(), .failure(URLError(.notConnectedToInternet))])

        let read = await model.read(Self.nvda)
        XCTAssertTrue(read)
        XCTAssertEqual(try screen(model).waiting.first?.unread, false)
        let sent = await transport.sent.last
        XCTAssertEqual(sent?.path, path(Self.nvda, "read"))

        let (failing, _, _) = try await loaded([.failure(URLError(.notConnectedToInternet))])
        let failed = await failing.read(Self.nvda)
        XCTAssertFalse(failed)
        XCTAssertEqual(try screen(failing).waiting.first?.unread, true)
    }

    func testReadingAnAlreadyReadRowStaysReadWhenTheCallFails() async throws {
        let (model, _, _) = try await loaded([.failure(URLError(.notConnectedToInternet))])
        let readKey = "proposal:00000000-0000-7000-8000-00000000b005"

        let ok = await model.read(readKey)

        XCTAssertFalse(ok)
        XCTAssertEqual(try screen(model).results.last?.unread, false)
    }

    func testKeepPutsOutTheDotAndPostsKeep() async throws {
        let (model, transport, _) = try await loaded([noContent()])

        let ok = await model.keep(Self.nvda)

        XCTAssertTrue(ok)
        XCTAssertEqual(try screen(model).waiting.first?.unread, false)
        let sent = await transport.sent.last
        XCTAssertEqual(sent?.path, path(Self.nvda, "keep"))
    }

    func testAFailedKeepLightsTheDotAgainAndAMissingKeyIsRefused() async throws {
        let (model, _, _) = try await loaded([.failure(URLError(.notConnectedToInternet))])
        let failed = await model.keep(Self.nvda)
        XCTAssertFalse(failed)
        XCTAssertEqual(try screen(model).waiting.first?.unread, true)
        let missing = await model.keep("nope")
        XCTAssertFalse(missing)
    }

    func testAReplayedReadReusesItsKeyAfterATransportError() async throws {
        let (model, transport, _) = try await loaded([.failure(URLError(.networkConnectionLost)), noContent()])

        _ = await model.read(Self.nvda)
        _ = await model.read(Self.nvda)

        let keys = try await idempotencyKeys(transport)
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAHintDuringAWriteReloadsOnceAfterItSettles() async throws {
        let (model, transport, hints) = try await loaded([.gate, .json(.ok, try Self.body())])
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(1)
        model.setVisible(true)

        let task = Task { await model.clear(Self.tsla) }
        await transport.waitForRequests(2)
        await hints.send(.changed(.user("me"), what: "catch_up_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let midCount = await transport.sent.count
        XCTAssertEqual(midCount, 2)
        await transport.releaseGate(noContent())
        _ = await task.value
        for _ in 0..<500 where await transport.sent.count < 3 { await Task.yield() }
        observer.cancel()

        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testFixturesAreCaughtUpAndFirstUse() {
        let caught = CatchUpScreen(.caughtUp)
        XCTAssertTrue(caught.isCaughtUp)
        XCTAssertEqual(caught.cleared.count, 6)
        XCTAssertEqual(caught.cabals.count, 3)
        let first = CatchUpScreen(.firstUse)
        XCTAssertTrue(first.isCaughtUp)
        XCTAssertTrue(first.cabals.isEmpty)
        XCTAssertTrue(first.cleared.isEmpty)
    }
}
