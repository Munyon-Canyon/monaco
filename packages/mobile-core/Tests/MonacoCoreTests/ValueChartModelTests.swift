import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ValueChartModelTests: XCTestCase {
    private typealias MyHistory = Components.Schemas.MyPnlHistory

    func testLoadShowsTheDefaultRange() async throws {
        let (model, transport, _) = try make([.curve(._1d)])

        await model.load()

        XCTAssertEqual(model.range, .oneDay)
        XCTAssertEqual(model.curve, ValueCurve(MyHistory.sample(range: ._1d)))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/pnl-history?range=1D"])
    }

    func testSelectingARangeLoadsItAndKeepsTheChipOnIt() async throws {
        let (model, _, _) = try make([.curve(._1d), .curve(._1w)])
        await model.load()

        await model.select(.oneWeek)

        XCTAssertEqual(model.range, .oneWeek)
        XCTAssertEqual(model.curve, ValueCurve(MyHistory.sample(range: ._1w)))
    }

    func testARangeTheScreenDoesNotOfferIsIgnored() async throws {
        let (model, transport, _) = try make([.curve(._1d)], ranges: [.oneDay, .oneWeek])
        await model.load()

        await model.select(.oneHour)

        XCTAssertEqual(model.range, .oneDay)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testAFailedRangeChangeKeepsTheCurveGoesBackToTheChipAndToasts() async throws {
        let (model, _, _) = try make([.curve(._1d), .failure])
        await model.load()
        let shown = model.curve

        await model.select(.oneWeek)

        XCTAssertEqual(model.curve, shown)
        XCTAssertEqual(model.range, .oneDay)
        XCTAssertEqual(model.toast, "You're offline. Try again.")
    }

    func testAFailedFirstLoadIsTheErrorStateAndTryAgainLoads() async throws {
        let (model, _, _) = try make([.failure, .curve(._1d)])

        await model.load()
        guard case .failed(.transport) = model.state else { return XCTFail("want a transport failure") }
        XCTAssertNil(model.toast)
        await model.load()

        XCTAssertNotNil(model.curve)
    }

    func testAUserHintRefreshesTheCurveOnScreen() async throws {
        let (model, transport, hints) = try make([.curve(._1d), .curve(._1d, shifted: true)])
        await model.load()
        let before = model.curve
        let task = Task { await model.observe() }
        while await hints.subscriberCount < 2 { await Task.yield() }

        await hints.send(.changed(.user("u1"), what: "balance", id: "1"))
        await transport.waitForRequests(2)
        while model.curve == before { await Task.yield() }

        XCTAssertNotEqual(model.curve, before)
        XCTAssertEqual(model.range, .oneDay)
        task.cancel()
    }

    func testChangingTheSubjectsReloadsTheChart() async throws {
        let (model, transport, _) = try make([.curve(._1d), .cabal(._1d)])
        await model.load()

        await model.setSubjects([.cabal(id: "cabal-1")])

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/cabals/cabal-1/value-history?range=1D")
        XCTAssertNotNil(model.curve)
    }

    private enum Script {
        case curve(MyHistory.RangePayload, shifted: Bool = false)
        case cabal(Components.Schemas.CabalValueHistory.RangePayload)
        case failure

        func reply() throws -> StubTransport.Reply {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            switch self {
            case .curve(let range, let shifted):
                var history = MyHistory.sample(range: range)
                if shifted { history.points[3].equityMicros += 1_000_000 }
                return .json(.ok, String(decoding: try encoder.encode(history), as: UTF8.self))
            case .cabal(let range):
                let history = Components.Schemas.CabalValueHistory.sample(cabalID: "cabal-1", range: range)
                return .json(.ok, String(decoding: try encoder.encode(history), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(
        _ script: [Script], ranges: [LeaderboardRange] = LeaderboardRange.allCases
    ) throws -> (ValueChartModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ValueChartModel(subjects: [.me], ranges: ranges, range: .oneDay, api: api, hints: hints)
        return (model, transport, hints)
    }
}
