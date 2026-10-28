import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelStartTests: XCTestCase {
    func testStartReadsTheAssetOneChartAndTheCabalsBeforeAnyAnswerArrives() async {
        let transport = StubTransport(scripted: [.hang, .hang, .hang])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())

        let started = Task { await model.start() }
        await transport.waitForRequests(3)
        started.cancel()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(
            paths.filter { $0.hasPrefix("/v1/assets/AAPLx/chart") }.count, 1, "no second chart call for stats")
        XCTAssertEqual(paths.filter { $0 == "/v1/assets/AAPLx" }.count, 1)
        XCTAssertEqual(paths.filter { $0 == "/v1/me/cabals" }.count, 1)
    }

    func testStatsComeFromTheAssetReadAndMakeNoChartCall() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [try reply(Components.Schemas.AssetDetail.nvda)],
            "/v1/assets/NVDAx/chart?range=1D": [try reply(chart(range: ._1d, interval: ._15m, prices: [100, 113]))],
            "/v1/me/cabals": [.json(.ok, "[]")],
        ])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = AssetDetailClientModel(api: api, symbol: "NVDAx", hints: FakeHintStream())

        await model.start()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.filter { $0.hasPrefix("/v1/assets/NVDAx/chart") }.count, 1)
        XCTAssertEqual(
            model.stats?.cells.map(\.value), ["$5.38T", "$9.4M", "$211.17", "$86.62", "$224.90", "1 cabal"])
    }

    func testAllNullStatsStillMakeSixDashes() async throws {
        let model = try modelReading(.nvdaDashes)

        await model.load()

        let cells = try XCTUnwrap(model.stats?.cells)
        XCTAssertEqual(cells.map(\.value), Array(repeating: "—", count: 6))
        XCTAssertEqual(cells.map(\.spokenValue), Array(repeating: "not available", count: 6))
    }

    func testAPreIpoAssetHasNoMarketStats() async throws {
        let model = try modelReading(.spaceX)

        await model.load()

        XCTAssertNil(model.stats)
    }

    private func modelReading(_ detail: Components.Schemas.AssetDetail) throws -> AssetDetailClientModel {
        let transport = StubTransport(routes: [
            "/v1/assets/\(detail.symbol)": [try reply(detail)],
            "/v1/assets/\(detail.symbol)/chart?range=1D": [
                try reply(chart(range: ._1d, interval: ._15m, prices: [100, 113]))
            ],
        ])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return AssetDetailClientModel(api: api, symbol: detail.symbol, hints: FakeHintStream())
    }

    private func reply<T: Encodable>(_ value: T) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func chart(
        range: Components.Schemas.AssetChart.RangePayload, interval: Components.Schemas.AssetChart.IntervalPayload,
        prices: [Int64]
    ) -> Components.Schemas.AssetChart {
        .init(
            range: range, interval: interval, bucketSeconds: 300,
            points: prices.enumerated().map { index, price in
                .init(
                    t: Date(timeIntervalSince1970: 1_772_596_200 + Double(index)), openMicros: price * 1_000_000,
                    highMicros: price * 1_000_000, lowMicros: price * 1_000_000, closeMicros: price * 1_000_000)
            },
            empty: false, attribution: "Test", nextBefore: nil
        )
    }
}
