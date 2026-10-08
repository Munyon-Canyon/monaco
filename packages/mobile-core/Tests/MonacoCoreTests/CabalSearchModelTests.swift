import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalSearchModelTests: XCTestCase {
    private let samples = Components.Schemas.CabalSearchItem.samples

    func testAnEmptyQueryIsIdleAndSendsNothing() async {
        let transport = StubTransport(scripted: [])
        let model = makeModel(transport)
        model.query = "   "
        await model.search()
        XCTAssertFalse(model.isActive)
        XCTAssertEqual(model.state, .idle)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testRowsReadTheirSizeModeAndAction() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(samples)))
        let model = makeModel(transport)
        model.query = " week "
        await model.search()
        guard case .rows(let rows) = model.state else { return XCTFail("expected rows, got \(model.state)") }
        XCTAssertEqual(
            rows.map(\.detail),
            [
                "3 members · Approval required", "1 member · Approval required",
                "5 members · Approval required", "2 members · Approval required",
            ]
        )
        XCTAssertEqual(rows.map(\.action), [.request, .request, .requested, .member])
        let sent = await transport.sent
        let query = URLComponents(string: sent.first?.path ?? "")?.queryItems ?? []
        XCTAssertEqual(URLComponents(string: sent.first?.path ?? "")?.path, "/v1/cabals")
        XCTAssertTrue(query.contains(URLQueryItem(name: "query", value: "week")))
        XCTAssertTrue(query.contains(URLQueryItem(name: "limit", value: "20")))
    }

    func testNoMatchNamesTheQuery() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.page([]))))
        model.query = "zzqq"
        await model.search()
        XCTAssertEqual(model.state, .empty("zzqq"))
    }

    func testATypedQueryNotYetSearchedIsLoading() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.page([]))))
        model.query = "zzqq"
        await model.search()
        model.query = "zzqqx"
        XCTAssertEqual(model.state, .loading)
    }

    func testAFailedFirstPageIsAnError() async {
        let model = makeModel(StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))]))
        model.query = "week"
        await model.search()
        guard case .failed(let error) = model.state else { return XCTFail("expected failure, got \(model.state)") }
        XCTAssertEqual(ToastCopy.message(for: error), "You're offline. Try again.")
        XCTAssertNil(model.toast)
    }

    func testAFailedNextPageKeepsTheRowsAndToasts() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        model.query = "week"
        await model.search()
        XCTAssertTrue(model.canLoadMore)
        await model.loadMore()
        guard case .rows(let rows) = model.state else { return XCTFail("expected rows, got \(model.state)") }
        XCTAssertEqual(rows.count, 2)
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertTrue(paths.last?.contains("cursor=c2") ?? false)
    }

    func testRequestingARowShowsRequestSent() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .json(.created, #"{"id":"r","direction":"request","status":"pending"}"#),
        ])
        let model = makeModel(transport)
        model.query = "week"
        await model.search()
        let row = rows(model)[1]
        let opened = await model.enter(row)
        XCTAssertNil(opened)
        XCTAssertEqual(model.toast?.message, "Request sent. You'll be in once the creator says yes.")
        XCTAssertEqual(rows(model)[1].action, .requested)
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/cabals/\(row.id)/access-requests")
    }

    func testARowAlreadyRequestedOrJoinedSendsNothing() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(samples)))
        let model = makeModel(transport)
        model.query = "week"
        await model.search()
        let requested = await model.enter(rows(model)[2])
        let member = await model.enter(rows(model)[3])
        XCTAssertNil(requested)
        XCTAssertNil(member)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testARefusalToastsTheServerMessage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            Self.problem(409, "cabal_banned", "This cabal was banned."),
        ])
        let model = makeModel(transport)
        model.query = "week"
        await model.search()
        let opened = await model.enter(rows(model)[0])
        XCTAssertNil(opened)
        XCTAssertEqual(model.toast?.message, "This cabal was banned.")
        XCTAssertEqual(model.toast?.isSuccess, false)
        XCTAssertEqual(rows(model)[0].action, .request)
    }

    func testAnApprovalRefreshesTheResults() async throws {
        var approved = samples[2]
        approved.isMember = true
        approved.myAccessRequestStatus = nil
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .json(.ok, try Self.page([samples[0], samples[1], approved, samples[3]])),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        model.query = "week"
        await model.search()
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }
        await hints.send(.changed(.user("me"), what: "cabal_access", id: "x"))
        let refreshed = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refreshed)
        _ = await waitUntil { await MainActor.run { self.rows(model)[2].action == .member } }
        XCTAssertEqual(rows(model)[2].action, .member)
    }

    func testThePreviewFiltersByName() async {
        let model = CabalSearchModel.preview()
        model.query = "warriors"
        await model.search()
        XCTAssertEqual(rows(model).map(\.name), ["Weekend warriors"])
        let opened = await model.enter(rows(model)[0])
        XCTAssertNil(opened)
        XCTAssertEqual(rows(model)[0].action, .requested)
    }

    private func rows(_ model: CabalSearchModel) -> [CabalSearchRow] {
        if case .rows(let rows) = model.state { return rows }
        return []
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CabalSearchModel {
        CabalSearchModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ items: [Components.Schemas.CabalSearchItem], next: String? = nil) throws -> String {
        let page = Components.Schemas.CabalSearchPage(items: items, nextCursor: next)
        return String(decoding: try JSONEncoder().encode(page), as: UTF8.self)
    }

    private static func cabal() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(Components.Schemas.Cabal.sample(role: "member")), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}
