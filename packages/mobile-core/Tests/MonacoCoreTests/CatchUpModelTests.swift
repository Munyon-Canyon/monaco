import Foundation
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
}
