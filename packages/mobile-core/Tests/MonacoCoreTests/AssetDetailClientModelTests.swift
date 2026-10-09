import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelTests: XCTestCase {
    func testLoadsTheGeneratedAssetDetail() async {
        let model = makeModel(
            StubTransport.Reply.json(.ok, detail),
            StubTransport.Reply.json(.ok, chart)
        )

        await model.load()

        XCTAssertEqual(model.phase, AssetDetailClientModel.Phase.loaded)
        XCTAssertEqual(model.detail?.name, "Apple")
        XCTAssertEqual(model.detail?.ticker, "AAPL")
        XCTAssertEqual(model.detail?.priceMicros, 110_000_000)
        XCTAssertEqual(model.detail?.changeBasisPoints, 1000)
        XCTAssertEqual(model.detail?.attribution, "Data provided by CoinGecko")
        XCTAssertEqual(model.detail?.otherListings.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.detail?.otherListings.map(\.name), ["Apple"])
        XCTAssertEqual(model.detail?.otherListings.first?.issuer, .prestocks)
        XCTAssertEqual(model.detail?.otherListings.first?.issuer.displayName, "PreStocks")
        XCTAssertEqual(model.chart?.range, .oneDay)
        XCTAssertEqual(model.chart?.points.map(\.priceUsdcMicros), [110_000_000, 111_000_000])
    }

    func testShowsTheDayRangeChangeAndLabelFromTheChart() async throws {
        let model = makeModel(
            try reply(Components.Schemas.AssetDetail.googl),
            try reply(chart(range: "1D", prices: [100_000_000, 101_250_000]))
        )

        await model.load()

        XCTAssertEqual(model.rangeChange?.basisPoints, 125)
        XCTAssertEqual(model.rangeChange?.label, "Past day · GOOGL")
    }

    func testShowsTheWeekRangeChangeAndLabelFromTheChart() async throws {
        let model = makeModel(
            try reply(Components.Schemas.AssetDetail.googl),
            try reply(chart(range: "1W", prices: [200_000_000, 198_000_000]))
        )

        await model.load()
        await model.loadChart(range: AssetChartRange.oneWeek)

        XCTAssertEqual(model.rangeChange?.basisPoints, -100)
        XCTAssertEqual(model.rangeChange?.label, "Past week · GOOGL")
    }

    func testShowsNoChangeUntilTheChartForTheSelectedRangeArrives() async throws {
        let transport = StubTransport(scripted: [
            try reply(Components.Schemas.AssetDetail.googl),
            try reply(chart(range: "1D", prices: [100_000_000, 101_250_000])),
            .gate,
        ])
        let model = makeModel(transport: transport, hints: FakeHintStream())
        await model.load()

        let week = Task { await model.loadChart(range: .oneWeek) }
        await transport.waitForRequests(3)

        XCTAssertEqual(model.rangeChange?.label, "Past week · GOOGL")
        XCTAssertNil(model.rangeChange?.basisPoints, "the day's percent must not sit beside the week's caption")

        await transport.releaseGate(try reply(chart(range: "1W", prices: [200_000_000, 198_000_000])))
        await week.value

        XCTAssertEqual(model.rangeChange?.basisPoints, -100)
    }

    func testReloadingKeepsTheChosenRangeAndTheDrawnChart() async throws {
        let transport = StubTransport(scripted: [
            try reply(Components.Schemas.AssetDetail.googl),
            try reply(chart(range: "1D", prices: [100_000_000, 101_250_000])),
            try reply(chart(range: "3M", prices: [100_000_000, 120_000_000])),
            try reply(Components.Schemas.AssetDetail.googl),
            .gate,
        ])
        let model = makeModel(transport: transport, hints: FakeHintStream())
        await model.load()
        await model.loadChart(range: .threeMonths)

        let back = Task { await model.load() }
        await transport.waitForRequests(5)

        XCTAssertEqual(model.selectedRange, .threeMonths)
        XCTAssertEqual(model.chartPhase, .loaded, "coming back must not swap the drawn chart for a skeleton")
        XCTAssertEqual(model.chart?.range, .threeMonths)

        await transport.releaseGate(try reply(chart(range: "3M", prices: [100_000_000, 125_000_000])))
        await back.value

        XCTAssertEqual(model.selectedRange, .threeMonths)
        XCTAssertEqual(model.chart?.points.last?.priceUsdcMicros, 125_000_000)
    }

    func testReloadingKeepsTheDrawnChartWhenTheChartReadFails() async throws {
        let model = makeModel(
            try reply(Components.Schemas.AssetDetail.googl),
            try reply(chart(range: "1D", prices: [100_000_000, 101_250_000])),
            try reply(Components.Schemas.AssetDetail.googl),
            StubTransport.Reply.failure(URLError(.cannotLoadFromNetwork))
        )
        await model.load()

        await model.load()

        XCTAssertEqual(model.chartPhase, .loaded)
        XCTAssertEqual(model.chart?.points.count, 2)
    }

    func testStatsReuseTheDrawnDayChartInsteadOfReadingItAgain() async throws {
        let transport = StubTransport(scripted: [
            try reply(Components.Schemas.AssetDetail.googl),
            StubTransport.Reply.json(.ok, chart),
            try reply(chart(range: "1Y", prices: [90_000_000, 150_000_000])),
        ])
        let model = makeModel(transport: transport, hints: FakeHintStream())
        await model.load()

        await model.loadStats()

        let sent = await transport.sent.count
        XCTAssertEqual(sent, 3, "the day chart is drawn already, so only the year is read")
        XCTAssertEqual(model.stats?.dayHigh, 113_000_000)
        XCTAssertEqual(model.stats?.yearHigh, 150_000_000)
    }

    func testRecognizesShortHistory() async throws {
        let model = makeModel(
            try reply(Components.Schemas.AssetDetail.googl), try reply(Components.Schemas.AssetChart.oneDay)
        )

        await model.load()

        XCTAssertTrue(model.isShortHistory)
    }

    func testMapsEverySessionFromTheDetail() async throws {
        for session in [MarketSession.open, .preMarket, .afterHours, .closed] {
            let detail = Components.Schemas.AssetDetail(
                symbol: "GOOGLx", displayName: "Alphabet xStock", issuer: .xstocks, kind: .equity,
                logoUrl: nil, priceMicros: 1, priceAsOf: nil, changeBps: nil, sparklineMicros: [],
                session: .init(
                    state: marketState(for: session), continuous: false,
                    holiday: "", earlyClose: false),
                decimals: 8, uiMultiplier: .init(num: 1, den: 1), tradable: true, otherListings: [], attribution: "Test"
            )
            let model = makeModel(try reply(detail), try reply(Components.Schemas.AssetChart.empty))

            await model.load()

            XCTAssertEqual(model.detail?.session, session)
        }
    }

    func testKeepsTheNewestChartRangeWhenAnEarlierRequestFinishesFirst() async {
        let transport = StubTransport(scripted: [.gate, .json(.ok, chart(range: "1Y"))])
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: transport
        )
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())

        let week = Task { await model.loadChart(range: .oneWeek) }
        while await transport.sent.count < 1 { await Task.yield() }
        let year = Task { await model.loadChart(range: .oneYear) }
        await year.value
        await transport.releaseGate(.json(.ok, chart(range: "1W")))
        await week.value
        await year.value

        XCTAssertEqual(model.selectedRange, .oneYear)
        XCTAssertEqual(model.chart?.range, .oneYear)
    }

    func testKeepsTheFailureForRetry() async {
        let model = makeModel(StubTransport.Reply.failure(URLError(.cannotConnectToHost)))

        await model.load()

        if case .failed = model.phase {} else { XCTFail("phase = \(model.phase)") }
        XCTAssertNotNil(model.lastError)
    }

    func testKeepsDetailWhenItsChartFails() async {
        let model = makeModel(
            StubTransport.Reply.json(.ok, detail),
            StubTransport.Reply.failure(URLError(.cannotLoadFromNetwork))
        )

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.detail?.ticker, "AAPL")
        XCTAssertNotNil(model.chartError)
        if case .failed = model.chartPhase {} else { XCTFail("chartPhase = \(model.chartPhase)") }
    }

    func testSelectsEachChartRangeBeforeReadingIt() async {
        let model = makeModel(StubTransport.Reply.json(.ok, chart(range: "1Y")))

        await model.loadChart(range: .oneYear)

        XCTAssertEqual(model.selectedRange, .oneYear)
        XCTAssertEqual(model.chart?.range, .oneYear)
    }

    func testKeepsTheChartLoadedWhenItIsEmpty() async throws {
        let model = makeModel(
            try reply(Components.Schemas.AssetDetail.googl), try reply(Components.Schemas.AssetChart.empty)
        )

        await model.load()

        XCTAssertEqual(model.chartPhase, AssetDetailClientModel.Phase.loaded)
        XCTAssertEqual(model.chart?.points.count, 0)
        XCTAssertNil(model.rangeChange?.basisPoints)
    }

    private func makeModel(_ responses: StubTransport.Reply...) -> AssetDetailClientModel {
        makeModel(transport: StubTransport(scripted: responses), hints: FakeHintStream())
    }

    private func makeModel(transport: StubTransport, hints: FakeHintStream) -> AssetDetailClientModel {
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: transport
        )
        return AssetDetailClientModel(api: api, symbol: "AAPLx", hints: hints)
    }

    private let detail = #"""
        {"symbol":"AAPLx","display_name":"Apple xStock","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":110000000,"price_as_of":null,"change_bps":1000,"sparkline_micros":[],"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":"after_hours","next_transition":null},"decimals":8,"ui_multiplier":{"num":1,"den":1},"tradable":true,"other_listings":[{"symbol":"AAPLx","display_name":"Apple xStock","issuer":"prestocks","kind":"equity","logo_url":null,"tradable":false}],"attribution":"Data provided by CoinGecko"}
        """#

    private let chart = #"""
        {"range":"1D","bucket_seconds":300,"points":[{"t":"2026-03-04T14:30:00Z","open_micros":100000000,"high_micros":112000000,"low_micros":99000000,"close_micros":110000000},{"t":"2026-03-04T14:35:00Z","open_micros":110000000,"high_micros":113000000,"low_micros":109000000,"close_micros":111000000}],"empty":false,"attribution":"Data provided by CoinGecko"}
        """#

    private func chart(range: String) -> String {
        #"""
        {"range":"\#(range)","bucket_seconds":300,"points":[],"empty":true,"attribution":"Data provided by CoinGecko"}
        """#
    }

    private func chart(range: String, prices: [Int64]) -> Components.Schemas.AssetChart {
        .init(
            range: chartRange(for: range), bucketSeconds: 300,
            points: prices.enumerated().map { index, price in
                .init(
                    t: Date(timeIntervalSince1970: 1_772_596_200 + Double(index)), openMicros: price,
                    highMicros: price, lowMicros: price, closeMicros: price)
            },
            empty: prices.isEmpty, attribution: "Test"
        )
    }

    private func reply<T: Encodable>(_ value: T) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func marketState(for session: MarketSession) -> Components.Schemas.MarketState {
        switch session {
        case .open: .open
        case .preMarket: .preMarket
        case .afterHours: .afterHours
        case .closed, .unknown: .closed
        }
    }

    private func chartRange(for range: String) -> Components.Schemas.AssetChart.RangePayload {
        switch range {
        case "1D": ._1d
        case "1W": ._1w
        case "1M": ._1m
        case "3M": ._3m
        case "1Y": ._1y
        default: .all
        }
    }
}
