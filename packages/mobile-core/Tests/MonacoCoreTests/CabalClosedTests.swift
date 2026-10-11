import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalClosedTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testABannedCabalIsClosed() async throws {
        let model = try await loaded(.sampleBanned(role: "member"))
        XCTAssertTrue(model.isClosed)
    }

    func testAnActiveCabalIsNotClosed() async throws {
        let model = try await loaded(.sample(role: "member"))
        XCTAssertFalse(model.isClosed)
    }

    func testAHintRefetchesOnceAndTheCabalCloses() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.json(.sample(role: "member"))),
            .json(.ok, try Self.json(.sampleBanned(role: "member"))),
        ])
        let hints = FakeHintStream()
        let model = CabalClosedModel(api: api(transport), hints: hints, cabalID: cabalID)
        await model.load()
        XCTAssertFalse(model.isClosed)
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "1"))
        let refetched = await waitUntil { await transport.sent.count == 2 && model.isClosed }
        XCTAssertTrue(refetched)
        for _ in 0..<50 { await Task.yield() }
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testAResyncRefetches() async throws {
        let transport = StubTransport(.json(.ok, try Self.json(.sample(role: "member"))))
        let hints = FakeHintStream()
        let model = CabalClosedModel(api: api(transport), hints: hints, cabalID: cabalID)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.resync)
        let refetched = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refetched)
    }

    func testAFailedReadKeepsTheLastState() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.json(.sampleBanned(role: "member"))),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = CabalClosedModel(api: api(transport), hints: FakeHintStream(), cabalID: cabalID)
        await model.load()
        await model.load()
        XCTAssertTrue(model.isClosed)
    }

    private func loaded(_ cabal: Components.Schemas.Cabal) async throws -> CabalClosedModel {
        let transport = StubTransport(.json(.ok, try Self.json(cabal)))
        let model = CabalClosedModel(api: api(transport), hints: FakeHintStream(), cabalID: cabalID)
        await model.load()
        return model
    }

    private static func json(_ cabal: Components.Schemas.Cabal) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(cabal), as: UTF8.self)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
