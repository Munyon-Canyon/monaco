import Foundation
import MonacoAPI
import MonacoTestSupport
import Observation
import Synchronization
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

    func testARefreshReadsTheCurveAgainAndShowsTheNewOne() async throws {
        let (model, transport, _) = try make([.curve(._1d), .curve(._1d, shifted: true)])
        await model.load()
        let before = model.curve

        await model.refresh()

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        XCTAssertNotEqual(model.curve, before)
        XCTAssertEqual(model.curve, ValueCurve(Self.shifted(MyHistory.sample(range: ._1d))))
    }

    func testAFailedRefreshKeepsTheCurveAndToasts() async throws {
        let (model, _, _) = try make([.curve(._1d), .failure])
        await model.load()
        let shown = model.curve

        await model.refresh()

        XCTAssertEqual(model.curve, shown)
        XCTAssertEqual(model.range, .oneDay)
        XCTAssertEqual(model.toast, "You're offline. Try again.")
    }

    func testChangingTheSubjectsReloadsTheChart() async throws {
        let (model, transport, _) = try make([.curve(._1d), .cabal(._1d)])
        await model.load()

        await model.setSubjects([.cabal(id: "cabal-1")])

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/cabals/cabal-1/value-history?range=1D")
        XCTAssertNotNil(model.curve)
    }

    func testTheShownRangeStaysOnTheOldRangeUntilTheNewOnesDataLands() async throws {
        let (model, transport, _) = try make([.curve(._1d), .gate])
        await model.load()
        XCTAssertEqual(model.shownRange, .oneDay)

        let selecting = Task { await model.select(.oneWeek) }
        await transport.waitForRequests(2)

        XCTAssertEqual(model.range, .oneWeek)
        XCTAssertEqual(model.shownRange, .oneDay)
        XCTAssertEqual(model.curve, ValueCurve(MyHistory.sample(range: ._1d)))
        await transport.releaseGate(try Script.curve(._1w).reply())
        await selecting.value
        XCTAssertEqual(model.shownRange, .oneWeek)
        XCTAssertEqual(model.curve, ValueCurve(MyHistory.sample(range: ._1w)))
    }

    func testSubjectsThatOnlySwapPlacesLeaveTheChartAlone() async throws {
        let (model, transport, _) = try make(
            [.cabal(._1d, id: "a"), .cabal(._1d, id: "b")], subjects: [.cabal(id: "a"), .cabal(id: "b")])
        await model.load()
        let shown = model.curves
        let touched = Mutex(false)
        withObservationTracking {
            _ = model.state
        } onChange: {
            touched.withLock { $0 = true }
        }

        await model.setSubjects([.cabal(id: "b"), .cabal(id: "a")])

        XCTAssertFalse(touched.withLock { $0 })
        XCTAssertEqual(model.curves, shown)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testNewSubjectsKeepTheLoadedChartWhileTheirCurvesLoad() async throws {
        let (model, transport, _) = try make([.cabal(._1d, id: "a"), .gate], subjects: [.cabal(id: "a")])
        await model.load()
        let before = model.curves

        let changing = Task { await model.setSubjects([.cabal(id: "a"), .cabal(id: "b")]) }
        await transport.waitForRequests(2)

        guard case .loaded = model.state else { return XCTFail("want the old curves on screen, got \(model.state)") }
        XCTAssertEqual(model.curves, before)
        await transport.releaseGate(try Script.cabal(._1d, id: "b").reply())
        await changing.value
        XCTAssertEqual(model.curves?.keys.count, 2)
    }

    private nonisolated static func shifted(_ history: MyHistory) -> MyHistory {
        var history = history
        history.points[3].equityMicros += 1_000_000
        return history
    }

    private enum Script {
        case curve(MyHistory.RangePayload, shifted: Bool = false)
        case cabal(Components.Schemas.CabalValueHistory.RangePayload, id: String = "cabal-1")
        case failure
        case gate

        func reply() throws -> StubTransport.Reply {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            switch self {
            case .curve(let range, let shifted):
                var history = MyHistory.sample(range: range)
                if shifted { history = ValueChartModelTests.shifted(history) }
                return .json(.ok, String(decoding: try encoder.encode(history), as: UTF8.self))
            case .cabal(let range, let id):
                let history = Components.Schemas.CabalValueHistory.sample(cabalID: id, range: range)
                return .json(.ok, String(decoding: try encoder.encode(history), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            case .gate:
                return .gate
            }
        }
    }

    private func make(
        _ script: [Script], ranges: [LeaderboardRange] = LeaderboardRange.allCases,
        subjects: [PnLHistoryLoader.Subject] = [.me]
    ) throws -> (ValueChartModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ValueChartModel(subjects: subjects, ranges: ranges, range: .oneDay, api: api, hints: hints)
        return (model, transport, hints)
    }
}
