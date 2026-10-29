import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProfileCabalsModelTests: XCTestCase {
    private func json<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private func cabals() throws -> String {
        try json([
            Components.Schemas.MyCabal(
                id: Components.Schemas.CabalRef.sampleAlpha.id, name: "Alpha", pictureUrl: nil, role: "admin",
                canVote: true, memberCount: 3, joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0,
                unreadCount: 0)
        ])
    }

    private func make(_ routes: [String: [StubTransport.Reply]]) -> (ProfileCabalsModel, StubTransport, FakeHintStream)
    {
        let transport = StubTransport(routes: routes)
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        return (ProfileCabalsModel(api: api, hints: hints), transport, hints)
    }

    func testLoadReadsTheThreeEndpoints() async throws {
        let (model, transport, _) = make([
            "/v1/me/cabals": [.json(.ok, try cabals())],
            "/v1/me/portfolio": [.json(.ok, try json(Components.Schemas.MyPortfolio.sample))],
            "/v1/me/cabal-invites": [.json(.ok, try json(Components.Schemas.CabalInvite.samples))],
        ])
        await model.load()
        XCTAssertEqual(model.rows.count, 3)
        XCTAssertEqual(model.memberCount, 1)
        let paths = await transport.sent.compactMap(\.path).sorted()
        XCTAssertEqual(paths, ["/v1/me/cabal-invites", "/v1/me/cabals", "/v1/me/portfolio"])
    }

    func testAFailedPortfolioStillListsMemberships() async throws {
        let (model, _, _) = make([
            "/v1/me/cabals": [.json(.ok, try cabals())],
            "/v1/me/portfolio": [.failure(URLError(.notConnectedToInternet))],
            "/v1/me/cabal-invites": [.json(.ok, "[]")],
        ])
        await model.load()
        guard case .member(let row) = model.rows.first else { return XCTFail("want a member row") }
        XCTAssertNil(row.value)
        XCTAssertEqual(row.role, "admin")
    }

    func testAFailedMembershipReadIsTheErrorState() async throws {
        let (model, _, _) = make([
            "/v1/me/cabals": [.failure(URLError(.notConnectedToInternet))],
            "/v1/me/portfolio": [.json(.ok, try json(Components.Schemas.MyPortfolio.sample))],
            "/v1/me/cabal-invites": [.json(.ok, "[]")],
        ])
        await model.load()
        guard case .failed = model.state else { return XCTFail("want failed") }
    }

    func testEachHintReloads() async throws {
        let round: [String: [StubTransport.Reply]] = [
            "/v1/me/cabals": [.json(.ok, try cabals()), .json(.ok, try cabals()), .json(.ok, try cabals())],
            "/v1/me/portfolio": [
                .json(.ok, try json(Components.Schemas.MyPortfolio.sample)),
                .json(.ok, try json(Components.Schemas.MyPortfolio.sample)),
                .json(.ok, try json(Components.Schemas.MyPortfolio.sample)),
            ],
            "/v1/me/cabal-invites": [.json(.ok, "[]"), .json(.ok, "[]"), .json(.ok, "[]")],
        ]
        let (model, transport, hints) = make(round)
        await model.load()
        let task = Task { await model.observe() }
        await hints.waitForSubscribers(2)
        await hints.send(.changed(.user("u1"), what: "cabals", id: "1"))
        while await transport.sent.count < 6 { await Task.yield() }
        await hints.send(.changed(.user("u1"), what: "balance_changed", id: "2"))
        while await transport.sent.count < 9 { await Task.yield() }
        task.cancel()
        let count = await transport.sent.count
        XCTAssertEqual(count, 9)
    }
}
