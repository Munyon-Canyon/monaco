import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelPagingTests: XCTestCase {
    private let start: Int64 = 1_790_085_600

    private func page(
        range: Components.Schemas.AssetChart.RangePayload = ._1w,
        interval: Components.Schemas.AssetChart.IntervalPayload = ._1h,
        step: Int64 = 3_600,
        from first: Int,
        count: Int,
        nextBefore: Date? = nil
    ) -> Components.Schemas.AssetChart {
        .init(
            range: range, interval: interval, bucketSeconds: step,
            points: (first..<(first + count)).map { index in
                let price = 100_000_000 + Int64(index) * 100_000
                return .init(
                    t: Date(timeIntervalSince1970: TimeInterval(start + Int64(index) * step)),
                    openMicros: price, highMicros: price + 50_000, lowMicros: price - 50_000, closeMicros: price)
            },
            empty: false, attribution: "Test", nextBefore: nextBefore)
    }

    private func reply<T: Encodable>(_ value: T) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func makeModel(_ transport: StubTransport, barCap: Int = AssetDetailClientModel.maximumBars)
        -> AssetDetailClientModel
    {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream(), barCap: barCap)
    }

    private func paths(_ transport: StubTransport) async -> [String] {
        await transport.sent.map { $0.path ?? "" }
    }

    func testTheDefaultIntervalIsLeftOutOfTheQuery() async throws {
        let transport = StubTransport(scripted: [try reply(page(from: 0, count: 10))])
        let model = makeModel(transport)

        await model.loadChart(range: .oneWeek)

        let sent = await paths(transport)
        XCTAssertEqual(sent, ["/v1/assets/AAPLx/chart?range=1W"])
        XCTAssertEqual(model.selectedInterval, .h1)
    }

    func testAChosenIntervalIsSentAndSurvivesTheReload() async throws {
        let transport = StubTransport(scripted: [
            try reply(page(interval: ._15m, step: 900, from: 0, count: 10)),
            try reply(page(interval: ._15m, step: 900, from: 0, count: 12)),
        ])
        let model = makeModel(transport)

        await model.loadChart(range: .oneWeek, interval: .m15)

        XCTAssertEqual(model.selectedInterval, .m15)
        XCTAssertEqual(model.chart?.interval, .m15)
        XCTAssertEqual(model.chart?.intervalSeconds, 900)
        XCTAssertEqual(model.chartPhase, .loaded)
        let sent = await paths(transport)
        XCTAssertEqual(sent, ["/v1/assets/AAPLx/chart?range=1W&interval=15m"])
    }

    func testChoosingARangeResetsTheIntervalToThatRangesDefault() async throws {
        let transport = StubTransport(scripted: [
            try reply(page(interval: ._15m, step: 900, from: 0, count: 10)),
            try reply(page(range: ._1m, interval: ._4h, step: 14_400, from: 0, count: 10)),
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek, interval: .m15)

        await model.loadChart(range: .oneMonth)

        XCTAssertEqual(model.selectedInterval, .h4)
        let sent = await paths(transport)
        XCTAssertEqual(sent.last, "/v1/assets/AAPLx/chart?range=1M&interval=4h")
    }

    func testMappingCarriesTheIntervalAndTheCursor() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start))
        let series = MarketMapping.chart(page(interval: ._4h, step: 14_400, from: 0, count: 3, nextBefore: cursor))

        XCTAssertEqual(series.interval, .h4)
        XCTAssertEqual(series.nextBefore, cursor)
        XCTAssertEqual(series.range, .oneWeek)
        XCTAssertEqual(series.bars.count, 3)
        XCTAssertEqual(MarketMapping.chart(page(range: ._6m, interval: ._1d, from: 0, count: 2)).range, .sixMonths)
    }

    func testLoadOlderPrependsTheOlderPageAndAdoptsItsCursor() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 10 * 3_600))
        let older = Date(timeIntervalSince1970: TimeInterval(start))
        let transport = StubTransport(scripted: [
            try reply(page(from: 10, count: 10, nextBefore: cursor)),
            try reply(page(from: 0, count: 10, nextBefore: older)),
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        await model.loadOlder()

        XCTAssertEqual(model.chart?.bars.count, 20)
        XCTAssertEqual(model.chart?.nextBefore, older)
        XCTAssertEqual(model.chartPhase, .loaded)
        XCTAssertFalse(model.isLoadingOlder)
        let sent = await paths(transport)
        XCTAssertEqual(sent.count, 2)
        XCTAssertTrue(sent[1].hasPrefix("/v1/assets/AAPLx/chart?range=1W&before=2026-"), sent[1])
        XCTAssertTrue(sent[1].hasSuffix("&limit=500"), sent[1])
        XCTAssertFalse(sent[0].contains("limit"), sent[0])
    }

    func testLoadOlderDoesNothingWithoutACursor() async throws {
        let transport = StubTransport(scripted: [try reply(page(from: 0, count: 10))])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        await model.loadOlder()

        let sent = await paths(transport)
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(model.chart?.bars.count, 10)
    }

    func testLoadOlderDropsAPageWhoseRangeChangedMeanwhile() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 10 * 3_600))
        let transport = StubTransport(scripted: [
            try reply(page(from: 10, count: 10, nextBefore: cursor)),
            .gate,
            try reply(page(range: ._1m, from: 0, count: 5)),
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        let older = Task { await model.loadOlder() }
        await transport.waitForRequests(2)
        await model.loadChart(range: .oneMonth)
        await transport.releaseGate(try reply(page(from: 0, count: 10)))
        await older.value

        XCTAssertEqual(model.chart?.range, .oneMonth)
        XCTAssertEqual(model.chart?.bars.count, 5)
        XCTAssertFalse(model.isLoadingOlder)
    }

    func testLoadOlderIsOneRequestAtATime() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 10 * 3_600))
        let transport = StubTransport(scripted: [
            try reply(page(from: 10, count: 10, nextBefore: cursor)),
            .gate,
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        let first = Task { await model.loadOlder() }
        await transport.waitForRequests(2)
        XCTAssertTrue(model.isLoadingOlder)
        await model.loadOlder()
        await transport.releaseGate(try reply(page(from: 0, count: 10)))
        await first.value

        let sent = await paths(transport)
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(model.chart?.bars.count, 20)
    }

    func testTheProductionBarCapIs4000() {
        XCTAssertEqual(AssetDetailClientModel.maximumBars, 4_000)
    }

    func testLoadOlderStopsAtTheBarCap() async throws {
        let cap = 40
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 100 * 3_600))
        let below = StubTransport(scripted: [
            try reply(page(from: 10, count: cap - 1, nextBefore: cursor)),
            try reply(page(from: 0, count: 10)),
        ])
        let under = makeModel(below, barCap: cap)
        await under.loadChart(range: .oneWeek)
        await under.loadOlder()
        let belowSent = await paths(below)
        XCTAssertEqual(belowSent.count, 2)

        let capped = StubTransport(scripted: [try reply(page(from: 0, count: cap, nextBefore: cursor))])
        let full = makeModel(capped, barCap: cap)
        await full.loadChart(range: .oneWeek)
        await full.loadOlder()

        let sent = await paths(capped)
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(full.chart?.bars.count, cap)
    }

    func testALoadOlderFailureLeavesTheChartAndAllowsARetry() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 10 * 3_600))
        let transport = StubTransport(scripted: [
            try reply(page(from: 10, count: 10, nextBefore: cursor)),
            .json(.internalServerError, "{}"),
            try reply(page(from: 0, count: 10)),
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        await model.loadOlder()
        XCTAssertEqual(model.chart?.bars.count, 10)
        XCTAssertEqual(model.chartPhase, .loaded)
        XCTAssertFalse(model.isLoadingOlder)

        await model.loadOlder()
        XCTAssertEqual(model.chart?.bars.count, 20)
    }

    func testAFullPresetWindowFirstPageFitsUnderTheCapAndStillPagesOlder() async throws {
        let cap = 60
        let window = cap - 5
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + Int64(window) * 3_600))
        let transport = StubTransport(scripted: [
            try reply(page(range: ._3m, interval: ._1d, step: 86_400, from: 0, count: window, nextBefore: cursor)),
            try reply(page(range: ._3m, interval: ._1d, step: 86_400, from: window, count: 10)),
        ])
        let model = makeModel(transport, barCap: cap)
        await model.loadChart(range: .threeMonths)

        XCTAssertEqual(model.chart?.bars.count, window)
        await model.loadOlder()

        XCTAssertEqual(model.chart?.bars.count, window + 10)
        let sent = await paths(transport)
        XCTAssertEqual(sent.first, "/v1/assets/AAPLx/chart?range=3M&interval=1d")
        XCTAssertEqual(sent.count, 2)
    }

    func testAFailedOlderRequestFreesTheModelCountsTheFailureAndAllowsARetry() async throws {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start + 10 * 3_600))
        let transport = StubTransport(scripted: [
            try reply(page(from: 10, count: 10, nextBefore: cursor)),
            StubTransport.Reply.failure(URLError(.timedOut)),
            try reply(page(from: 0, count: 10)),
        ])
        let model = makeModel(transport)
        await model.loadChart(range: .oneWeek)

        await model.loadOlder()

        XCTAssertFalse(model.isLoadingOlder)
        XCTAssertEqual(model.olderFailureTick, 1)
        XCTAssertEqual(model.chart?.bars.count, 10)
        XCTAssertEqual(model.chartPhase, .loaded)

        await model.loadOlder()
        XCTAssertEqual(model.chart?.bars.count, 20)
        XCTAssertEqual(model.olderFailureTick, 1)
    }
}
