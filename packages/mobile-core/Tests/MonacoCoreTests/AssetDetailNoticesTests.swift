import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailNoticesTests: XCTestCase {
    func testAPriceOlderThanFiveMinutesIsStaleByItsOwnAge() async throws {
        let moment = TestMoment()
        let model = try await loaded(priceAsOf: moment.before(minutes: 14), moment: moment)

        XCTAssertEqual(model.notices, [.stale(minutes: 14)])
        XCTAssertEqual(model.notices.map(\.text), ["Token price delayed · 14 minutes ago."])
    }

    func testAPriceFromTheLastFiveMinutesIsFresh() async throws {
        let moment = TestMoment()
        let model = try await loaded(priceAsOf: moment.date.addingTimeInterval(-300), moment: moment)

        XCTAssertTrue(model.notices.isEmpty)
        XCTAssertNil(model.staleMinutes)
    }

    func testAPriceJustPastTheLimitIsStale() async throws {
        let moment = TestMoment()
        let model = try await loaded(priceAsOf: moment.date.addingTimeInterval(-301), moment: moment)

        XCTAssertEqual(model.staleMinutes, 5)
    }

    func testAnUnpricedAssetHasNothingToBeStaleAbout() async throws {
        let moment = TestMoment()
        let model = try await loaded(priceMicros: nil, priceAsOf: nil, moment: moment)

        XCTAssertNil(model.staleMinutes)
    }

    func testAFailedRefreshWithAPriceOnScreenIsStaleFromTheLastGoodLoad() async throws {
        let moment = TestMoment()
        let model = makeModel(
            [
                try Self.reply(priceAsOf: moment.date), .json(.ok, Self.emptyChart),
                .failure(URLError(.cannotConnectToHost)),
            ], moment: moment)
        await model.load()
        XCTAssertNil(model.staleMinutes)
        moment.advance(minutes: 3)

        await model.load()

        XCTAssertTrue(model.refreshFailed)
        XCTAssertEqual(model.notices, [.stale(minutes: 3)])
    }

    func testAGoodRefreshClearsTheNotice() async throws {
        let moment = TestMoment()
        let model = makeModel(
            [
                try Self.reply(priceAsOf: moment.date), .json(.ok, Self.emptyChart),
                .failure(URLError(.cannotConnectToHost)),
                try Self.reply(priceAsOf: moment.date), .json(.ok, Self.emptyChart),
            ], moment: moment)
        await model.load()
        await model.load()
        XCTAssertFalse(model.notices.isEmpty)

        await model.load()

        XCTAssertFalse(model.refreshFailed)
        XCTAssertTrue(model.notices.isEmpty)
    }

    func testAnAssetThatCannotTradeSaysSoAndAPausedOneSaysItIsPaused() async throws {
        let moment = TestMoment()
        let untradable = try await loaded(priceAsOf: moment.date, tradable: false, moment: moment)
        let paused = try await loaded(priceAsOf: moment.date, quotable: false, moment: moment)

        XCTAssertEqual(untradable.notices, [.unavailable])
        XCTAssertEqual(untradable.notices.map(\.text), ["This stock is unavailable for new proposals."])
        XCTAssertEqual(paused.notices, [.paused])
        XCTAssertEqual(paused.notices.map(\.text), ["Trading paused right now."])
    }

    func testStaleComesFirstWhenTwoNoticesApply() async throws {
        let moment = TestMoment()
        let model = try await loaded(priceAsOf: moment.before(minutes: 9), tradable: false, moment: moment)

        XCTAssertEqual(model.notices, [.stale(minutes: 9), .unavailable])
    }

    private func loaded(
        priceMicros: Int64? = 175_420_000, priceAsOf: Date?, tradable: Bool = true, quotable: Bool = true,
        moment: TestMoment
    ) async throws -> AssetDetailClientModel {
        let model = makeModel(
            [
                try Self.reply(
                    priceMicros: priceMicros, priceAsOf: priceAsOf, tradable: tradable, quotable: quotable),
                .json(.ok, Self.emptyChart),
            ], moment: moment)
        await model.load()
        return model
    }

    private func makeModel(_ replies: [StubTransport.Reply], moment: TestMoment) -> AssetDetailClientModel {
        AssetDetailClientModel(
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"),
                transport: StubTransport(scripted: replies)),
            symbol: "GOOGLx", hints: FakeHintStream(), now: moment.now)
    }

    private static let emptyChart = """
        {"range":"1D","interval":"5m","bucket_seconds":300,"points":[],"empty":true,"attribution":"x","next_before":null}
        """

    private static func reply(
        priceMicros: Int64? = 175_420_000, priceAsOf: Date?, tradable: Bool = true, quotable: Bool = true
    ) throws -> StubTransport.Reply {
        var detail = Components.Schemas.AssetDetail.googl
        detail.priceMicros = priceMicros
        detail.priceAsOf = priceAsOf
        detail.tradable = tradable
        detail.quotable = quotable
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))
    }
}
