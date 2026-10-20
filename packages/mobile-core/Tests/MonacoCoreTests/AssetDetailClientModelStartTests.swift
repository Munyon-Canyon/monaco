import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelStartTests: XCTestCase {
    func testStartReadsTheAssetAndBothChartsBeforeAnyAnswerArrives() async {
        let transport = StubTransport(scripted: [.hang, .hang, .hang, .hang])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())

        let started = Task { await model.start() }
        await transport.waitForRequests(4)
        started.cancel()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.filter { $0.hasPrefix("/v1/assets/AAPLx/chart") }.count, 2)
        XCTAssertEqual(paths.filter { $0 == "/v1/assets/AAPLx" }.count, 1)
        XCTAssertEqual(paths.filter { $0 == "/v1/me/cabals" }.count, 1)
    }

    func testStatsShowTheDayFiguresBeforeTheYearSeriesLands() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/AAPLx": [try reply(Components.Schemas.AssetDetail.googl)],
            "/v1/assets/AAPLx/chart?range=1D": [try reply(chart(range: ._1d, interval: ._15m, prices: [100, 113]))],
            "/v1/assets/AAPLx/chart?range=1Y": [.gate],
            "/v1/me/cabals": [.json(.ok, "[]")],
        ])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())

        let started = Task { await model.start() }
        await transport.waitForRequests(4)
        for _ in 0..<100_000 where model.stats == nil { await Task.yield() }

        XCTAssertEqual(model.stats?.dayHigh, 113_000_000)
        XCTAssertNil(model.stats?.yearHigh, "the year read is still out")

        await transport.releaseGate(try reply(chart(range: ._1y, interval: ._1d, prices: [90, 150])))
        await started.value

        XCTAssertEqual(model.stats?.dayHigh, 113_000_000)
        XCTAssertEqual(model.stats?.yearHigh, 150_000_000)
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
