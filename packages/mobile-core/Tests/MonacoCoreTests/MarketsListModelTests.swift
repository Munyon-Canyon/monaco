import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class MarketsListModelTests: XCTestCase {
    func testEachOrderSendsItsFilterAndSort() async throws {
        let expected: [(MarketsListModel.Order, filter: String, sort: String?)] = [
            (.name, "all", "name"), (.price, "all", "price"), (.movers, "movers", nil),
        ]
        for (order, filter, sort) in expected {
            let transport = StubTransport(.json(.ok, try Self.page(["AAPLx"])))
            let model = makeModel(transport, order: order)

            await model.load()

            let sent = await transport.sent
            let path = try XCTUnwrap(sent.first?.path)
            XCTAssertTrue(path.contains("filter=\(filter)"), path)
            XCTAssertEqual(path.contains("sort=\(sort ?? "")"), sort != nil, path)
            if sort == nil { XCTAssertFalse(path.contains("sort="), path) }
            XCTAssertTrue(path.contains("limit=\(MarketsListModel.pageLimit)"), path)
        }
    }

    func testPagesFollowNextCursorAndKeepTheServersOrder() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["ZZZx"], cursor: "c2")),
            .json(.ok, try Self.page(["MMMx"], cursor: "c3")),
            .json(.ok, try Self.page(["AAAx"])),
        ])
        let model = makeModel(transport, order: .name)

        await model.load()
        XCTAssertEqual(model.nextCursor, "c2")
        await model.loadMore()
        await model.loadMore()
        await model.loadMore()

        XCTAssertEqual(model.rows.map(\.symbol), ["ZZZx", "MMMx", "AAAx"])
        XCTAssertNil(model.nextCursor)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.count, 3)
        XCTAssertFalse(paths[0].contains("cursor="), paths[0])
        XCTAssertTrue(paths[1].contains("cursor=c2"), paths[1])
        XCTAssertTrue(paths[2].contains("cursor=c3"), paths[2])
    }

    func testSelectingAnOrderStartsOverFromTheFirstPage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["AAPLx"], cursor: "c2")),
            .json(.ok, try Self.page(["NVDAx"])),
        ])
        let model = makeModel(transport, order: .movers)
        await model.load()

        await model.select(.price)

        XCTAssertEqual(model.order, .price)
        XCTAssertEqual(model.rows.map(\.symbol), ["NVDAx"])
        XCTAssertNil(model.nextCursor)
        let sent = await transport.sent
        let path = try XCTUnwrap(sent.last?.path)
        XCTAssertTrue(path.contains("filter=all"), path)
        XCTAssertTrue(path.contains("sort=price"), path)
        XCTAssertFalse(path.contains("cursor="), path)
    }

    func testAFirstPageThatFailsIsAnErrorAndAFailedLaterPageKeepsTheRows() async throws {
        let failing = makeModel(StubTransport(.failure(URLError(.cannotConnectToHost))), order: .name)
        await failing.load()
        guard case .failed = failing.phase else { return XCTFail("expected failed, got \(failing.phase)") }

        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["AAPLx"], cursor: "c2")), .failure(URLError(.cannotConnectToHost)),
        ])
        let model = makeModel(transport, order: .name)
        await model.load()

        await model.loadMore()

        XCTAssertEqual(model.rows.map(\.symbol), ["AAPLx"])
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testTheSortMenuOffersNamePriceAndTodaysMovers() {
        XCTAssertEqual(MarketsListModel.Order.allCases.map(\.title), ["Name", "Price", "Today's movers"])
    }

    private func makeModel(_ transport: any ClientTransport, order: MarketsListModel.Order) -> MarketsListModel {
        MarketsListModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            order: order)
    }

    private static func page(_ symbols: [String], cursor: String? = nil) throws -> String {
        let assets = symbols.map { symbol -> Components.Schemas.AssetSummary in
            var asset = Components.Schemas.AssetSummary.googl
            asset.symbol = symbol
            asset.displayName = "\(symbol.dropLast()) xStock"
            return asset
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let list = Components.Schemas.AssetList(assets: assets, nextCursor: cursor)
        return String(decoding: try encoder.encode(list), as: UTF8.self)
    }
}

@MainActor
final class StocksTabMoversTests: XCTestCase {
    func testMoversComeFromTheMoversFilterInServerOrderAndThePreviewStopsAtFive() async throws {
        let symbols = ["TSLAx", "AMDx", "NFLXx", "AMZNx", "MSFTx", "NVDAx"]
        let transport = StubTransport(.json(.ok, try Self.page(symbols)))
        let model = makeModel(transport)

        await model.loadMarkets()

        let paths = await transport.sent.compactMap(\.path)
        let asked = try XCTUnwrap(paths.first { $0.contains("filter=movers") })
        XCTAssertTrue(asked.contains("limit=10"), asked)
        XCTAssertEqual(model.movers.map(\.symbol), symbols)
        XCTAssertEqual(model.moverPreview.map(\.symbol), Array(symbols.prefix(5)))
    }

    func testAPriceHintReloadsTheMovers() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(["TSLAx"])))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.loadMarkets()
        let observing = Task { await model.observe() }
        addTeardownBlock { observing.cancel() }
        for _ in 0..<500 where await hints.subscriberCount == 0 { await Task.yield() }
        let before = await transport.sent.compactMap(\.path).filter { $0.contains("filter=movers") }.count

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        var after = before
        for _ in 0..<500 where after == before {
            await Task.yield()
            after = await transport.sent.compactMap(\.path).filter { $0.contains("filter=movers") }.count
        }
        XCTAssertEqual(after, before + 1)
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> StocksTabModel {
        StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: TestClock(),
            watchlist: WatchlistModel(userID: "u1", store: InMemoryWatchlistStore([:])))
    }

    private static func page(_ symbols: [String]) throws -> String {
        let assets = symbols.map { symbol -> Components.Schemas.AssetSummary in
            var asset = Components.Schemas.AssetSummary.googl
            asset.symbol = symbol
            return asset
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(
            decoding: try encoder.encode(Components.Schemas.AssetList(assets: assets, nextCursor: nil)), as: UTF8.self)
    }
}
