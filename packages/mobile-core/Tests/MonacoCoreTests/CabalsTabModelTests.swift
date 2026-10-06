import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalsTabModelTests: XCTestCase {
    func testLoadSendsOneGetAndDecodesEveryRow() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.list(["Weekend pot", "Work pot"]))])
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/cabals"])
        XCTAssertEqual(sent.map(\.method), [.get])
        guard case .loaded(let cabals) = model.state else {
            XCTFail("expected rows, got \(model.state)")
            return
        }
        XCTAssertEqual(cabals.map(\.name), ["Weekend pot", "Work pot"])
        XCTAssertEqual(cabals.map(\.memberCount), [1, 2])
    }

    func testARefreshKeepsTheRowsUntilTheAnswerArrives() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.list(["Weekend pot"])), .gate])
        let model = CabalsTabModel(api: api(transport))
        await model.load()

        let refresh = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(started)
        XCTAssertEqual(model.state, .loaded(try rows(["Weekend pot"])))

        await transport.releaseGate(.json(.ok, Self.list(["Weekend pot", "QA pot"])))
        await refresh.value
        XCTAssertEqual(model.state, .loaded(try rows(["Weekend pot", "QA pot"])))
    }

    func testAFailedRefreshKeepsTheRowsAndRaisesAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.list(["Weekend pot"])),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = CabalsTabModel(api: api(transport))

        await model.load()
        await model.load()

        XCTAssertEqual(model.state, .loaded(try rows(["Weekend pot"])))
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    func testAFirstLoadThatFailsShowsTheServerMessage() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 503, code: .dbUnavailable,
                message: "We couldn't load your cabals.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true
            ))
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        guard case .failed(let error) = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertEqual(ToastCopy.message(for: error), "We couldn't load your cabals.")
        XCTAssertEqual(model.failureTick, 1)
    }

    func testAnOlderAnswerDoesNotOverwriteANewerOne() async throws {
        let transport = StubTransport(scripted: [.gate, .json(.ok, Self.list(["New"]))])
        let model = CabalsTabModel(api: api(transport))

        let first = Task { await model.load() }
        _ = await waitUntil { await transport.sent.count == 1 }
        await model.load()
        await transport.releaseGate(.json(.ok, Self.list(["Old"])))
        await first.value

        XCTAssertEqual(model.state, .loaded(try rows(["New"])))
    }

    func testMemberCopy() {
        XCTAssertEqual(CabalCopy.memberCount(1), "1 member")
        XCTAssertEqual(CabalCopy.memberCount(3), "3 members")
        let named = member(displayName: "Kai", handle: "kai", role: "creator")
        XCTAssertEqual(CabalCopy.memberName(named), "Kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: " ", handle: "kai", role: "member")), "@kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: "", handle: nil, role: "member")), "Member")
    }

    func testOnlyTheCreatorSeesPendingRequests() {
        XCTAssertEqual(CabalCopy.requestBadge(myCabal(role: "creator", pending: 2)), 2)
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "creator", pending: 0)))
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "member", pending: 2)))
        XCTAssertEqual(CabalCopy.requestCount(1), "1 request to join")
        XCTAssertEqual(CabalCopy.requestCount(2), "2 requests to join")
    }

    func testTheUnreadBadgeFormatterHidesZeroAndCapsAtNinetyNine() {
        XCTAssertNil(CabalCopy.unreadBadge(0))
        XCTAssertEqual(CabalCopy.unreadBadge(7), "7")
        XCTAssertEqual(CabalCopy.unreadBadge(99), "99")
        XCTAssertEqual(CabalCopy.unreadBadge(100), "99+")
        XCTAssertEqual(CabalCopy.unreadLabel(1), "1 unread message")
        XCTAssertEqual(CabalCopy.unreadLabel(3), "3 unread messages")
        XCTAssertEqual(CabalCopy.unreadLabel(100), "More than 99 unread messages")
    }

    func testACabalsHintReadsTheListAgain() async {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.list(["Weekend pot"])), .json(.ok, Self.list(["Weekend pot", "QA pot"])),
        ])
        let hints = FakeHintStream()
        let model = CabalsTabModel(api: api(transport))
        await model.load()
        let observer = Task { await model.observe(hints: hints) }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user("me"), what: "cabals", id: "1"))

        let refreshed = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refreshed)
        let both = await waitUntil { model.state == .loaded((try? self.rows(["Weekend pot", "QA pot"])) ?? []) }
        XCTAssertTrue(both)
    }

    func testAHintForAnotherTopicLeavesTheListAlone() async {
        let transport = StubTransport(scripted: [.json(.ok, Self.list(["Weekend pot"]))])
        let hints = FakeHintStream()
        let model = CabalsTabModel(api: api(transport))
        await model.load()
        let observer = Task { await model.observe(hints: hints) }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user("me"), what: "feed", id: "1"))

        _ = await waitUntil { false }
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testPullToRefreshRunsTheLatestReloadOfEverySection() async {
        let refresh = ScreenRefresh()
        var runs: [String] = []
        refresh.register("list") { runs.append("stale list") }
        refresh.register("list") { runs.append("list") }
        refresh.register("chart") { runs.append("chart") }

        await refresh.run()

        XCTAssertEqual(runs.sorted(), ["chart", "list"])
    }

    private func myCabal(role: String, pending: Int32) -> Components.Schemas.MyCabal {
        .init(
            id: "c-1", name: "QA pot", pictureUrl: nil, role: role, canVote: true, memberCount: 2,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: pending, unreadCount: 0)
    }

    private func member(displayName: String, handle: String?, role: String) -> Components.Schemas.CabalMember {
        .init(
            userId: "u-1", handle: handle, displayName: displayName, photoUrl: nil, role: role, canVote: true,
            joinedAt: Date(timeIntervalSince1970: 0))
    }

    private func rows(_ names: [String]) throws -> [Components.Schemas.MyCabal] {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode([Components.Schemas.MyCabal].self, from: Data(Self.list(names).utf8))
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

    private static func list(_ names: [String]) -> String {
        let rows = names.enumerated().map { index, name in
            """
            {"id":"01890a5d-ac96-774b-bcce-b302099a80\(60 + index)","name":"\(name)","picture_url":null,"role":"creator","can_vote":true,"member_count":\(index + 1),"joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}
            """
        }
        return "[\(rows.joined(separator: ","))]"
    }
}
