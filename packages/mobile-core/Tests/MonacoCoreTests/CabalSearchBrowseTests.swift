import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalSearchBrowseTests: XCTestCase {
    private let samples = Components.Schemas.CabalSearchItem.samples

    func testAnEmptyQueryBrowsesEveryPublicCabal() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(samples)))
        let model = makeModel(transport)
        XCTAssertTrue(model.needsSearch)
        XCTAssertEqual(model.state, .loading)
        model.query = "   "
        await model.search()
        XCTAssertFalse(model.needsSearch)
        XCTAssertEqual(rows(model).count, samples.count)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        let query = URLComponents(string: sent.first?.path ?? "")?.queryItems ?? []
        XCTAssertTrue(query.contains(URLQueryItem(name: "query", value: "")))
        XCTAssertTrue(query.contains(URLQueryItem(name: "limit", value: "20")))
    }

    func testNoPublicCabalsAtAllIsEmptyWithNoQuery() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.page([]))))
        await model.search()
        XCTAssertEqual(model.state, .empty(""))
    }

    func testBrowsingPagesByCursor() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c1")),
            .json(.ok, try Self.page(Array(samples.suffix(2)))),
        ])
        let model = makeModel(transport)
        await model.search()
        XCTAssertEqual(rows(model).count, 2)
        XCTAssertTrue(model.canLoadMore)
        await model.loadMore()
        XCTAssertEqual(rows(model).map(\.id), samples.map(\.id))
        let sent = await transport.sent
        let second = URLComponents(string: sent.last?.path ?? "")?.queryItems ?? []
        XCTAssertTrue(second.contains(URLQueryItem(name: "cursor", value: "c1")))
    }

    func testClearingTheQueryGoesBackToBrowsing() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(1)))),
            .json(.ok, try Self.page(samples)),
        ])
        let model = makeModel(transport)
        model.query = "week"
        await model.search()
        XCTAssertEqual(rows(model).count, 1)
        model.query = ""
        XCTAssertTrue(model.needsSearch)
        await model.search()
        XCTAssertEqual(rows(model).count, samples.count)
        XCTAssertFalse(model.needsSearch)
    }

    private func rows(_ model: CabalSearchModel) -> [CabalSearchRow] {
        if case .rows(let rows) = model.state { return rows }
        return []
    }

    private func makeModel(_ transport: StubTransport) -> CabalSearchModel {
        CabalSearchModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream()
        )
    }

    private static func page(_ items: [Components.Schemas.CabalSearchItem], next: String? = nil) throws -> String {
        let page = Components.Schemas.CabalSearchPage(items: items, nextCursor: next)
        return String(decoding: try JSONEncoder().encode(page), as: UTF8.self)
    }
}
