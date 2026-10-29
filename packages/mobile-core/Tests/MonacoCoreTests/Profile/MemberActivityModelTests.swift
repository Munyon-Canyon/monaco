import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class MemberActivityModelTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func stamp(_ ago: TimeInterval) -> String {
        ISO8601DateFormatter().string(from: now.addingTimeInterval(-ago))
    }

    private func cabal() -> String { #"{"id":"c1","name":"AC Capital","picture_url":null}"# }

    private func page(next: String?) -> String {
        let cursor = next.map { "\"\($0)\"" } ?? "null"
        return """
            {"items":[
            {"kind":"voted","at":"\(stamp(1200))","title":"Voted","cabal":\(cabal()),"proposal_id":"p1","subject":"Buy NVDA","symbol":"NVDA","usdc_micros":null,"choice":"yes"},
            {"kind":"proposed","at":"\(stamp(18000))","title":"Proposed","cabal":\(cabal()),"proposal_id":"p2","subject":"Buy META","symbol":"META","usdc_micros":null,"status":"passed","result":{"yes":4,"no":1}},
            {"kind":"replied","at":"\(stamp(259_200))","cabal":\(cabal()),"comment_id":"m1","proposal_id":"p3","body":"Concentration","subject":"Buy NVDA","usdc_micros":1500000000},
            {"kind":"topped_board","at":"\(stamp(1_814_400))","title":"Topped the board","period_start":"2026-09-01","right":11,"resolved":12}
            ],"next_cursor":\(cursor)}
            """
    }

    private func make(_ replies: [StubTransport.Reply]) -> (MemberActivityModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        let now = now
        return (MemberActivityModel(userID: "u1", api: api, now: { now }), transport)
    }

    func testEachKindsCopyAndRelativeTimes() async {
        let (model, transport) = make([.json(.ok, page(next: nil))])
        await model.load()
        XCTAssertEqual(
            model.rows.map(\.title),
            ["Voted Yes", "Proposed Buy META", "“Concentration”", "Topped the board"])
        XCTAssertEqual(
            model.rows.map(\.subtitle),
            [
                "Buy NVDA · AC Capital", "AC Capital · passed 4–1", "on Buy NVDA $1,500.00 · AC Capital",
                "September · 11 of 12 right",
            ])
        XCTAssertEqual(model.rows.map(\.time), ["20m", "5h", "3d", "3w"])
        let path = await transport.sent.first?.path
        XCTAssertTrue(path?.hasPrefix("/v1/users/u1/activity") == true)
    }

    func testLoadMoreSendsTheCursorAndAppends() async {
        let (model, transport) = make([.json(.ok, page(next: "abc")), .json(.ok, page(next: nil))])
        await model.load()
        await model.loadMore()
        XCTAssertEqual(model.rows.count, 8)
        XCTAssertEqual(Set(model.rows.map(\.id)).count, 8)
        let path = await transport.sent.last?.path
        XCTAssertTrue(path?.contains("cursor=abc") == true)
    }

    func testFailuresLeaveNoRowsOrKeepLoadedRows() async {
        let (empty, _) = make([.failure(URLError(.notConnectedToInternet))])
        await empty.load()
        guard case .failed = empty.state else { return XCTFail("want failed") }
        let (model, _) = make([.json(.ok, page(next: "abc")), .failure(URLError(.notConnectedToInternet))])
        await model.load()
        await model.loadMore()
        XCTAssertEqual(model.rows.count, 4)
    }
}
