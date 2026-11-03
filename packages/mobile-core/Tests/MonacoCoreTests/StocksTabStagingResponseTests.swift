import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import OpenAPIRuntime
import Synchronization
import XCTest

@MainActor
final class StocksTabStagingResponseTests: XCTestCase {
    func testAStagingAllResponseDecodesIntoTheStocksTheListScreenShows() async throws {
        let transport = FixtureTransport()
        let model = MarketsListModel(api: makeAPI(transport), order: .name)

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.rows.count, 20)
        XCTAssertTrue(model.rows.allSatisfy { $0.kind == .stock })
        XCTAssertEqual(model.rows.first?.symbol, "A5Gx")
        XCTAssertNotNil(model.nextCursor)
    }

    func testMarketsShowsStagingStocksWithPreIpoAndMovers() async throws {
        let transport = FixtureTransport()
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": []]))
        let model = StocksTabModel(
            api: makeAPI(transport), hints: FakeHintStream(), clock: TestClock(), watchlist: watchlist)

        await model.loadMarkets()

        XCTAssertEqual(model.preIpo.phase, .loaded)
        XCTAssertEqual(model.preIpo.rows.count, 11)
        XCTAssertEqual(model.preIpoPreview.count, StocksTabModel.sectionLimit)
        XCTAssertEqual(model.stocks.phase, .loaded)
        XCTAssertEqual(model.stocksPreview.count, StocksTabModel.sectionLimit)
        XCTAssertTrue(model.stocksPreview.allSatisfy { $0.kind == .stock })
        XCTAssertEqual(model.moverPreview.count, StocksTabModel.sectionLimit)
        XCTAssertTrue(model.moverPreview.allSatisfy { $0.kind == .stock })
        XCTAssertTrue(
            transport.sent.contains { $0.hasPrefix("/v1/assets?filter=popular") }, "\(transport.sent)")
    }

    func testAFailedStocksReadLeavesPreIpoAndMoversOnScreen() async throws {
        let transport = FixtureTransport(failing: "filter=popular")
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": []]))
        let model = StocksTabModel(
            api: makeAPI(transport), hints: FakeHintStream(), clock: TestClock(), watchlist: watchlist)

        await model.loadMarkets()

        XCTAssertTrue(model.stocksPreview.isEmpty)
        XCTAssertEqual(model.preIpo.phase, .loaded)
        XCTAssertEqual(model.preIpo.rows.count, 11)
        XCTAssertEqual(model.moverPreview.count, StocksTabModel.sectionLimit)
    }

    private func makeAPI(_ transport: any ClientTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}

private final class FixtureTransport: ClientTransport, Sendable {
    private let paths = Mutex<[String]>([])
    private let failing: String?

    init(failing: String? = nil) {
        self.failing = failing
    }

    var sent: [String] { paths.withLock { $0 } }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        let path = request.path ?? ""
        paths.withLock { $0.append(path) }
        if let failing, path.contains(failing) { throw URLError(.notConnectedToInternet) }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let name: String
        if path.contains("filter=pre_ipo") {
            name = "assets_pre_ipo_staging"
        } else if path.contains("filter=movers") || path.contains("filter=popular") {
            name = "assets_movers_staging"
        } else if path.contains("filter=all") {
            name = "assets_all_staging"
        } else {
            return (response, HTTPBody(Data(#"{"facts":[]}"#.utf8)))
        }
        let url = try XCTUnwrap(Bundle.module.url(forResource: name, withExtension: "json"))
        return (response, HTTPBody(try Data(contentsOf: url)))
    }
}
