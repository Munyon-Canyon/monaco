import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalMembersModelTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private static let joined = Date(timeIntervalSince1970: 1_790_000_000)

    func testAHundredAndTwentyMembersPageUntilTheCursorEndsWithoutRepeats() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(0..<50, next: "c1", totals: (2, 80, 38))),
            .json(.ok, try Self.page(50..<100, next: "c2", totals: (2, 80, 38))),
            .json(.ok, try Self.page(100..<120, next: nil, totals: (2, 80, 38))),
        ])
        let model = makeModel(transport, role: "member")

        await model.loadFirst()
        XCTAssertEqual(model.rows.count, 50)
        XCTAssertEqual(model.totals?.voters, 80)
        XCTAssertEqual(CabalMemberCopy.sectionTitle(.voter, totals: try XCTUnwrap(model.totals)), "Voters 80")
        XCTAssertTrue(model.hasMore)

        await model.loadMore(after: model.rows[10])
        XCTAssertEqual(model.rows.count, 50)

        await model.loadMore(after: try XCTUnwrap(model.rows.last))
        await model.loadMore(after: try XCTUnwrap(model.rows.last))

        let ids = model.rows.map(\.id)
        XCTAssertEqual(ids.count, 120)
        XCTAssertEqual(Set(ids).count, 120)
        XCTAssertFalse(model.hasMore)
        XCTAssertEqual(model.preview.count, 5)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertTrue(paths[0].contains("order=role_share"))
        XCTAssertTrue(paths[1].contains("cursor=c1"))
    }

    func testTheCrownShowsOnExactlyTheCrownedRow() async throws {
        let model = makeModel(
            StubTransport(.json(.ok, try Self.encode(Components.Schemas.CabalMemberPage.sampleACCapital))),
            role: "member")
        await model.loadFirst()
        XCTAssertEqual(model.rows.filter(\.row.crown).map(\.row.displayName), ["Christelle"])
        XCTAssertEqual(model.rows(in: .admin).map(\.row.displayName), ["Andres", "Christelle"])
        XCTAssertEqual(model.voting.count, 8)
        XCTAssertTrue(model.notVoting.isEmpty)
    }

    func testANonAdminNeverLoadsRequests() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Components.Schemas.CabalMemberPage.sampleACCapital)))
        let model = makeModel(transport, role: "member")
        await model.loadFirst()
        XCTAssertTrue(model.requests.isEmpty)
        XCTAssertFalse(model.isAdmin)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.count, 1)
        XCTAssertFalse(paths.contains { $0.contains("access-requests") })
        await model.approve("anything")
        let after = await transport.sent.count
        XCTAssertEqual(after, 1)
    }

    func testAnAdminLoadsRequestsWithTheLinkAndAge() async throws {
        let model = makeModel(try adminTransport(), role: "admin")
        await model.loadFirst()
        XCTAssertEqual(model.requests.map(\.name), ["Leo", "@priya"])
        XCTAssertEqual(model.requests.first?.via, "Jordan")
        XCTAssertNil(model.requests.last?.via)
        XCTAssertTrue(model.canPropose)
    }

    func testApprovingPostsOnceReloadsAndToastsTheJoin() async throws {
        let transport = StubTransport(
            routes: try Self.adminRoutes(
                decision: .json(.ok, #"{"id":"r","direction":"request","status":"approved"}"#)))
        let model = makeModel(transport, role: "creator")
        await model.loadFirst()
        let leo = try XCTUnwrap(model.requests.first)

        await model.approve(leo.id)

        XCTAssertEqual(model.toast?.message, "Leo joined AC Capital.")
        XCTAssertEqual(model.toast?.isSuccess, true)
        XCTAssertEqual(model.requests.map(\.name), ["@priya"])
        let sent = await transport.sent
        let decisions = sent.filter { ($0.path ?? "").hasSuffix("/decision") }
        XCTAssertEqual(decisions.count, 1)
        let bodies = await transport.sentBodies
        let index = try XCTUnwrap(sent.firstIndex { ($0.path ?? "").hasSuffix("/decision") })
        let body = try XCTUnwrap(bodies[index])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "approve"])
        XCTAssertTrue(model.deciding.isEmpty)
    }

    func testDecliningToastsDeclined() async throws {
        let transport = StubTransport(
            routes: try Self.adminRoutes(
                decision: .json(.ok, #"{"id":"r","direction":"request","status":"denied"}"#)))
        let model = makeModel(transport, role: "admin")
        await model.loadFirst()
        let leo = try XCTUnwrap(model.requests.first)

        await model.deny(leo.id)

        XCTAssertEqual(model.toast?.message, "Declined.")
        XCTAssertEqual(model.requests.map(\.name), ["@priya"])
    }

    func testARequestSomeoneElseDecidedShowsTheServerMessageAndReloads() async throws {
        let transport = StubTransport(
            routes: try Self.adminRoutes(
                decision: Self.problem(409, "access_request_not_pending", "Someone already answered.")))
        let model = makeModel(transport, role: "admin")
        await model.loadFirst()
        let leo = try XCTUnwrap(model.requests.first)

        await model.approve(leo.id)

        XCTAssertEqual(model.toast?.message, "Someone already answered.")
        XCTAssertEqual(model.toast?.isSuccess, false)
        XCTAssertEqual(model.requests.map(\.name), ["@priya"])
        let count = await transport.sent.count
        XCTAssertEqual(count, 5)
    }

    func testAnUnknownRequestSendsNothing() async throws {
        let transport = try adminTransport()
        let model = makeModel(transport, role: "admin")
        await model.loadFirst()
        let before = await transport.sent.count

        await model.approve("missing")

        let after = await transport.sent.count
        XCTAssertEqual(after, before)
        XCTAssertNil(model.toast)
    }

    func testAFailedFirstPageSurfacesTheFailure() async throws {
        let model = makeModel(StubTransport(.failure(URLError(.notConnectedToInternet))), role: "member")
        await model.loadFirst()
        guard case .failed = model.phase else { return XCTFail("got \(model.phase)") }
        XCTAssertTrue(model.rows.isEmpty)
        XCTAssertNil(model.totals)
    }

    func testRetryingMoreFetchesTheNextPage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(0..<2, next: "c1", totals: (1, 3, 0))),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, try Self.page(2..<4, next: nil, totals: (1, 3, 0))),
        ])
        let model = makeModel(transport, role: "member")
        await model.loadFirst()
        await model.loadMore(after: try XCTUnwrap(model.rows.last))
        guard case .failed = model.phase else { return XCTFail("got \(model.phase)") }

        await model.retryMore()

        XCTAssertEqual(model.rows.count, 4)
    }

    func testMemberAndAccessHintsReloadAndOtherCabalsDoNot() async throws {
        let page = try Self.encode(Components.Schemas.CabalMemberPage.sampleACCapital)
        let transport = StubTransport(.json(.ok, page))
        let hints = FakeHintStream()
        let model = makeModel(transport, role: "member", hints: hints)
        await model.loadFirst()
        model.setVisible(true)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }
        let base = await transport.sent.count

        await hints.send(.changed(.cabal(Self.cabalID), what: "members", id: "x"))
        let reloaded = await waitUntil { await transport.sent.count == base + 1 }
        XCTAssertTrue(reloaded)

        await hints.send(.changed(.cabal("other"), what: "members", id: "x"))
        for _ in 0..<50 { await Task.yield() }
        let after = await transport.sent.count
        XCTAssertEqual(after, base + 1)
    }

    private func adminTransport() throws -> StubTransport {
        StubTransport(routes: try Self.adminRoutes(decision: nil))
    }

    private static let membersPath = "/v1/cabals/\(cabalID)/members?limit=50&order=role_share"
    private static let requestsPath = "/v1/cabals/\(cabalID)/access-requests"

    private static func adminRoutes(decision: StubTransport.Reply?) throws -> [String: [StubTransport.Reply]] {
        let page = StubTransport.Reply.json(.ok, try encode(Components.Schemas.CabalMemberPage.sampleACCapital))
        let all = StubTransport.Reply.json(.ok, try encode(Components.Schemas.CabalAccessRequest.samples))
        let rest = StubTransport.Reply.json(.ok, try encode([Components.Schemas.CabalAccessRequest.samples[1]]))
        var routes = [membersPath: [page, page], requestsPath: [all, rest]]
        if let decision {
            routes[
                "/v1/cabals/\(cabalID)/access-requests/\(Components.Schemas.CabalAccessRequest.samples[0].id)/decision"] =
                [
                    decision
                ]
        }
        return routes
    }

    private func makeModel(
        _ transport: StubTransport, role: String, hints: FakeHintStream = FakeHintStream()
    ) -> CabalMembersModel {
        var cabal = Components.Schemas.Cabal.sample(role: role)
        cabal.id = Self.cabalID
        cabal.name = "AC Capital"
        return CabalMembersModel(
            cabal: cabal,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ range: Range<Int>, next: String?, totals: (Int32, Int32, Int32)) throws -> String {
        let rows = range.map { index in
            Components.Schemas.CabalMemberRow(
                userId: "u\(index)", handle: nil, displayName: "Member \(index)", photoUrl: nil, role: .member,
                canVote: true, joinedAt: joined, ownershipBps: 10,
                month: .init(right: 0, resolved: 0, voted: 0, eligible: 0),
                crown: false)
        }
        return try encode(
            Components.Schemas.CabalMemberPage(
                items: rows, nextCursor: next, totals: .init(admins: totals.0, voters: totals.1, members: totals.2)))
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }
}
