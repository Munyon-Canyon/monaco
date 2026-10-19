import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest
import os

@testable import MonacoCore

@MainActor
final class PortfolioStaleTests: XCTestCase {
    private struct Clock: Sendable {
        private let stored = OSAllocatedUnfairLock(initialState: Date(timeIntervalSince1970: 1_800_000_000))

        var date: Date {
            get { stored.withLock { $0 } }
            nonmutating set { stored.withLock { $0 = newValue } }
        }
    }

    func testALoadedPortfolioIsFreshAndStampsTheTime() async throws {
        let clock = Clock()
        let model = try make([.ok(.sample)], clock: clock)

        await model.load()

        XCTAssertEqual(model.lastUpdated, clock.date)
        XCTAssertFalse(model.isStale)
    }

    func testAFailedRefreshOverLoadedContentIsStaleAndKeepsTheTime() async throws {
        let clock = Clock()
        let model = try make([.ok(.sample), .failure], clock: clock)
        await model.load()
        let loadedAt = clock.date
        clock.date = clock.date.addingTimeInterval(14 * 60)

        await model.load()

        XCTAssertTrue(model.isStale)
        XCTAssertEqual(model.lastUpdated, loadedAt)
        XCTAssertEqual(model.summary, PortfolioSummary(.sample))
    }

    func testARecoveredRefreshClearsTheStaleFlag() async throws {
        let clock = Clock()
        let model = try make([.ok(.sample), .failure, .ok(.sampleLoss)], clock: clock)
        await model.load()
        await model.load()
        clock.date = clock.date.addingTimeInterval(60)

        await model.load()

        XCTAssertFalse(model.isStale)
        XCTAssertEqual(model.lastUpdated, clock.date)
    }

    func testAFailedFirstLoadIsNotStale() async throws {
        let model = try make([.failure], clock: Clock())

        await model.load()

        XCTAssertFalse(model.isStale)
        XCTAssertNil(model.lastUpdated)
    }

    func testTheStaleLineCountsWholeMinutes() {
        let then = Date(timeIntervalSince1970: 1_800_000_000)

        XCTAssertEqual(
            PortfolioStaleCopy.line(lastUpdated: then, now: then.addingTimeInterval(14 * 60 + 20)),
            "Last updated 14 minutes ago.")
        XCTAssertEqual(
            PortfolioStaleCopy.line(lastUpdated: then, now: then.addingTimeInterval(20)),
            "Last updated 1 minute ago.")
        XCTAssertEqual(PortfolioStaleCopy.line(lastUpdated: nil, now: then), "Last updated a while ago.")
    }

    func testNetWorthIsCabalSharesPlusCash() {
        let summary = PortfolioSummary(.sample)

        XCTAssertEqual(summary.netWorthMicros(cashMicros: 500_000_000), summary.totalMicros + 500_000_000)
        XCTAssertEqual(summary.netWorthMicros(cashMicros: 0), summary.totalMicros)
    }

    func testThePeriodCopyAndTheHomePeriods() {
        XCTAssertEqual(LeaderboardRange.homePeriods, [.oneWeek, .oneMonth, .all])
        XCTAssertEqual(LeaderboardRange.oneWeek.returnPeriodPhrase, "past week")
        XCTAssertEqual(LeaderboardRange.oneMonth.returnPeriodPhrase, "past month")
        XCTAssertEqual(LeaderboardRange.all.returnPeriodPhrase, "all time")
    }

    private enum Script {
        case ok(Components.Schemas.MyPortfolio)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .ok(let portfolio):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(portfolio), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(_ script: [Script], clock: Clock) throws -> PortfolioModel {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return PortfolioModel(api: api, hints: FakeHintStream(), now: { clock.date })
    }
}
