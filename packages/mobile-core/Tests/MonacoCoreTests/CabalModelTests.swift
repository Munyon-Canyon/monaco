import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testLoadSendsOneGetAndDecodesTheCabal() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.cabal(name: "Weekend pot", members: 1))])
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/cabals/\(cabalID)"])
        guard case .loaded(let cabal) = model.state else {
            XCTFail("expected the cabal, got \(model.state)")
            return
        }
        XCTAssertEqual(cabal.name, "Weekend pot")
        XCTAssertEqual(cabal.memberCount, 1)
        XCTAssertEqual(cabal.members.map(\.role), ["creator"])
    }

    func testAnUpdatedHintRefetchesTheCabal() async throws {
        try await assertRefetch(after: .changed(.cabal(cabalID), what: "updated", id: "1"))
    }

    func testAMembersHintRefetchesTheCabal() async throws {
        try await assertRefetch(after: .changed(.cabal(cabalID), what: "members", id: "1"))
    }

    func testAResyncRefetchesTheCabal() async throws {
        try await assertRefetch(after: .resync)
    }

    func testACashOutChangeRefetchesTheBoard() async throws {
        try await assertRefetch(after: .changed(.user("me"), what: "cashout_changed", id: "1"))
    }

    func testHintsForAnotherCabalOrAnotherTopicSendNothing() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(name: "Weekend pot", members: 1)))
        let hints = FakeHintStream()
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 3 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8061"), what: "updated", id: "1"))
        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8061"), what: "members", id: "2"))
        await hints.send(.changed(.cabal(cabalID), what: "proposals", id: "3"))
        await hints.send(.changed(.cabal(cabalID), what: "members", id: "4"))
        let marker = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(marker)
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAHiddenScreenWaitsToRefetchUntilItIsVisible() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(name: "Weekend pot", members: 1)))
        let hints = FakeHintStream()
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 3 }

        model.setVisible(false)
        await hints.send(.changed(.cabal(cabalID), what: "updated", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)

        model.setVisible(true)
        let refetched = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refetched)
    }

    func testAProblemBodyFailsTheScreenWithTheServerMessage() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 404, code: .cabalNotFound,
                message: "That cabal doesn't exist.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false
            ))
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()

        guard case .failed(let error) = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertEqual(ToastCopy.message(for: error), "That cabal doesn't exist.")
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError, error)
    }

    func testAFailedRefreshKeepsTheLoadedCabal() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.cabal(name: "Weekend pot", members: 1)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()
        await model.load()

        guard case .loaded(let cabal) = model.state else {
            XCTFail("expected the cabal kept, got \(model.state)")
            return
        }
        XCTAssertEqual(cabal.name, "Weekend pot")
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    private func assertRefetch(after hint: Hint, file: StaticString = #filePath, line: UInt = #line) async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.cabal(name: "Weekend pot", members: 1)),
            .json(.ok, Self.cabal(name: "Weekend pot", members: 2)),
        ])
        let hints = FakeHintStream()
        let model = CabalModel(cabalID: cabalID, api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 3 }
        XCTAssertTrue(subscribed, file: file, line: line)

        await hints.send(hint)
        let refetched = await waitUntil {
            guard case .loaded(let cabal) = model.state else { return false }
            return cabal.memberCount == 2
        }

        XCTAssertTrue(refetched, file: file, line: line)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, Array(repeating: "/v1/cabals/\(cabalID)", count: 2), file: file, line: line)
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

    static func cabal(name: String, members: Int) -> String {
        let people = (0..<members).map { index in
            let role = index == 0 ? "creator" : "member"
            return """
                {"user_id":"01890a5d-ac96-774b-bcce-b302099a80\(70 + index)","handle":"kai\(index)","display_name":"Kai \(index)","photo_url":null,"role":"\(role)","can_vote":true,"joined_at":"2026-09-30T12:00:00Z"}
                """
        }.joined(separator: ",")
        return """
            {"id":"01890a5d-ac96-774b-bcce-b302099a8060","name":"\(name)","picture_url":null,"status":"active",\
            "rules":{"join_mode":"open","voter_mode":"all","threshold":"majority","proposal_expiry_seconds":86400,"slippage_bps":100},\
            "creator":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8070","handle":"kai0","display_name":"Kai 0","photo_url":null},\
            "member_count":\(members),"members":[\(people)],"me":{"role":"creator","can_vote":true},\
            "my_access_request":null,"invite_code":"ABCD2345","treasury_address":"treasury-1"}
            """
    }
}
