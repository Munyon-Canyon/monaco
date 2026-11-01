import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailNoticesTests: XCTestCase {
    func testAnAssetThatCannotTradeSaysSoAndAPausedOneSaysItIsPaused() async throws {
        let moment = TestMoment()
        let untradable = try await loaded(priceAsOf: moment.date, tradable: false, moment: moment)
        let paused = try await loaded(priceAsOf: moment.date, quotable: false, moment: moment)

        XCTAssertEqual(untradable.notices, [.unavailable])
        XCTAssertEqual(untradable.notices.map(\.text), ["This stock is unavailable for new proposals."])
        XCTAssertEqual(paused.notices, [.paused])
        XCTAssertEqual(paused.notices.map(\.text), ["Trading paused right now."])
    }

    func testAStalePriceSaysDelayedAndAFreshOneSaysNothing() async throws {
        let moment = TestMoment()
        let flagged = try await loaded(priceAsOf: moment.date, stale: true, moment: moment)
        let live = try await loaded(priceAsOf: moment.date, stale: false, moment: moment)
        let paused = try await loaded(priceAsOf: moment.date, quotable: false, stale: true, moment: moment)

        XCTAssertEqual(flagged.notices, [.delayed])
        XCTAssertEqual(flagged.notices.map(\.text), ["Price delayed. This is the last price we have."])
        XCTAssertTrue(flagged.detail?.market.isDelayed == true)
        XCTAssertEqual(live.notices, [])
        XCTAssertEqual(paused.notices, [.paused])
    }

    private func loaded(
        priceMicros: Int64? = 175_420_000, priceAsOf: Date?, tradable: Bool = true, quotable: Bool = true,
        stale: Bool? = nil, moment: TestMoment
    ) async throws -> AssetDetailClientModel {
        let model = makeModel(
            [
                try Self.reply(
                    priceMicros: priceMicros, priceAsOf: priceAsOf, tradable: tradable, quotable: quotable,
                    stale: stale),
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
        priceMicros: Int64? = 175_420_000, priceAsOf: Date?, tradable: Bool = true, quotable: Bool = true,
        stale: Bool? = nil
    ) throws -> StubTransport.Reply {
        var detail = Components.Schemas.AssetDetail.googl
        detail.priceMicros = priceMicros
        detail.priceAsOf = priceAsOf
        detail.tradable = tradable
        detail.quotable = quotable
        detail.stale = stale
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))
    }
}
