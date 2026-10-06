import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalValueHistoryModelTests: XCTestCase {
    private typealias Pot = Components.Schemas.CabalValueHistory

    private static let alpha = Components.Schemas.CabalRef.sampleAlpha.id
    private static let beta = Components.Schemas.CabalRef.sampleBeta.id

    func testItReadsThePortfolioForTheCabalsThenOneCurvePerCabalAtOneMonth() async throws {
        let (model, transport, _) = try make([.portfolio(.sample), .pot(Self.alpha), .pot(Self.beta)])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.range, .oneMonth)
        XCTAssertEqual(model.lines.map(\.name), ["Alpha", "Sunday Investors"])
        XCTAssertEqual(model.ranges, [.oneDay, .oneWeek, .oneMonth, .all])
        let paths = await transport.sent.compactMap(\.path).sorted()
        XCTAssertEqual(
            paths,
            [
                "/v1/cabals/\(Self.alpha)/value-history?range=1M", "/v1/cabals/\(Self.beta)/value-history?range=1M",
                "/v1/me/portfolio",
            ])
    }

    func testAViewerWithNoCabalHidesTheChart() async throws {
        let (model, transport, _) = try make([.portfolio(.sampleEmpty)])

        await model.load()

        XCTAssertEqual(model.phase, .hidden)
        XCTAssertTrue(model.lines.isEmpty)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testAFailedPortfolioIsTheErrorStateAndTryAgainLoads() async throws {
        let (model, _, _) = try make([.failure, .portfolio(.sample), .pot(Self.alpha), .pot(Self.beta)])

        await model.load()
        XCTAssertEqual(model.phase, .failed)
        XCTAssertNil(model.toast)
        await model.load()

        XCTAssertEqual(model.phase, .loaded)
    }

    func testAFailedCurveReadIsTheErrorStateAndTryAgainReadsItAgain() async throws {
        var one = Components.Schemas.MyPortfolio.sample
        one.cabals.removeLast()
        let (model, _, _) = try make([.portfolio(one), .failure, .portfolio(one), .pot(Self.alpha)])

        await model.load()
        XCTAssertEqual(model.phase, .failed)
        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.lines.count, 1)
    }

    func testSelectingARangeReloadsEveryLine() async throws {
        let (model, transport, _) = try make([
            .portfolio(.sample), .pot(Self.alpha), .pot(Self.beta), .pot(Self.alpha, ._1w), .pot(Self.beta, ._1w),
        ])
        await model.load()

        await model.select(.oneWeek)

        XCTAssertEqual(model.range, .oneWeek)
        XCTAssertEqual(model.lines.count, 2)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.filter { $0.hasSuffix("range=1W") }.count, 2)
    }

    func testACabalCreatedTodayHasNoLineToDrawButTheOthersDo() async throws {
        let (model, _, _) = try make([.portfolio(.sample), .pot(Self.alpha), .short(Self.beta)])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertTrue(model.hasEnoughHistory)
        XCTAssertEqual(Set(model.lines.map(\.curve.hasEnoughHistory)), [true, false])
    }

    private enum Script {
        case portfolio(Components.Schemas.MyPortfolio)
        case pot(String, Pot.RangePayload = ._1m)
        case short(String)
        case failure

        func reply() throws -> StubTransport.Reply {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            switch self {
            case .portfolio(let portfolio):
                return .json(.ok, String(decoding: try encoder.encode(portfolio), as: UTF8.self))
            case .pot(let id, let range):
                return .json(
                    .ok, String(decoding: try encoder.encode(Pot.sample(cabalID: id, range: range)), as: UTF8.self))
            case .short(let id):
                return .json(.ok, String(decoding: try encoder.encode(Pot.sampleShort(cabalID: id)), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(_ script: [Script]) throws -> (CabalValueHistoryModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (CabalValueHistoryModel(api: api, hints: hints), transport, hints)
    }
}
