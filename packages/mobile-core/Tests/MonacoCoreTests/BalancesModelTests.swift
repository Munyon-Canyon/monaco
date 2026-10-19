import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class BalancesModelTests: XCTestCase {
    private enum Portfolio {
        case sample
        case empty
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .sample: try Self.json(.sample)
            case .empty: try Self.json(.sampleEmpty)
            case .failure: .failure(URLError(.notConnectedToInternet))
            }
        }

        private static func json(_ portfolio: Components.Schemas.MyPortfolio) throws -> StubTransport.Reply {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            return .json(.ok, String(decoding: try encoder.encode(portfolio), as: UTF8.self))
        }
    }

    private enum Cash {
        case micros(String)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .micros(let available):
                let balance = Components.Schemas.Balance(
                    availableMicros: available, onChainMicros: available, inFlightMicros: "0",
                    depositAddress: "wallet-1", asOf: Date(timeIntervalSince1970: 1_759_579_200))
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(balance), as: UTF8.self))
            case .failure: return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    func testReadyAddsTheCabalTotalToCashInMicros() async throws {
        let model = try make(portfolio: [.sample], cash: [.micros("500000000")])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.netWorthMicros, 1_500_000_000)
        XCTAssertEqual(model.netWorth, "$1,500.00")
        XCTAssertEqual(model.inCabals, "$1,000.00")
        XCTAssertEqual(model.cash, "$500.00")
        XCTAssertFalse(model.isCashOnly)
    }

    func testAllocationFollowsThePortfolioRowOrder() async throws {
        let model = try make(portfolio: [.sample], cash: [.micros("500000000")])

        await model.load()

        XCTAssertEqual(model.allocation.map(\.value), ["$600.00", "$400.00"])
        XCTAssertEqual(model.allocation.map(\.id), Components.Schemas.MyPortfolio.sample.cabals.map(\.cabal.id))
    }

    func testCashOnlyIsAPortfolioWithNoRows() async throws {
        let model = try make(portfolio: [.empty], cash: [.micros("500000000")])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertTrue(model.isCashOnly)
        XCTAssertEqual(model.netWorth, "$500.00")
        XCTAssertEqual(model.inCabals, "$0.00")
        XCTAssertTrue(model.allocation.isEmpty)
    }

    func testNothingLoadedYetIsLoadingWithNoFigures() throws {
        let model = try make(portfolio: [], cash: [])

        XCTAssertEqual(model.phase, .loading)
        XCTAssertNil(model.netWorthMicros)
        XCTAssertEqual(model.netWorth, BalancesModel.unavailable)
        XCTAssertFalse(model.isCashOnly)
    }

    func testAFailedPortfolioReadIsTheFailedPhase() async throws {
        let model = try make(portfolio: [.failure], cash: [.micros("500000000")])

        await model.load()

        XCTAssertEqual(model.phase, .failed)
        XCTAssertNil(model.netWorthMicros)
        XCTAssertNil(model.toast)
    }

    func testAFailedBalanceReadIsTheFailedPhase() async throws {
        let model = try make(portfolio: [.sample], cash: [.failure])

        await model.load()

        XCTAssertEqual(model.phase, .failed)
        XCTAssertNil(model.netWorthMicros)
        XCTAssertEqual(model.cash, BalancesModel.unavailable)
    }

    func testTryingAgainAfterAFailureLoadsBothReads() async throws {
        let model = try make(portfolio: [.failure, .sample], cash: [.micros("500000000"), .micros("500000000")])
        await model.load()

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.netWorth, "$1,500.00")
    }

    func testAFailedPortfolioRefreshKeepsTheFiguresAndToasts() async throws {
        let model = try make(
            portfolio: [.sample, .failure], cash: [.micros("500000000"), .micros("500000000")])
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.netWorth, "$1,500.00")
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testAFailedBalanceRefreshKeepsTheFiguresAndToasts() async throws {
        let model = try make(portfolio: [.sample, .sample], cash: [.micros("500000000"), .failure])
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.netWorth, "$1,500.00")
        XCTAssertNotNil(model.toast)
    }

    func testARefreshThatSucceedsLeavesNoToast() async throws {
        let model = try make(portfolio: [.sample, .sample], cash: [.micros("500000000"), .micros("700000000")])
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.netWorth, "$1,700.00")
        XCTAssertNil(model.toast)
    }

    func testThePreviewReadyScenarioHoldsTheReferenceFigures() async {
        let model = BalancesModel.preview(.ready)

        await model.load()

        XCTAssertEqual(model.netWorth, "$11,582.32")
        XCTAssertEqual(model.inCabals, "$11,082.32")
        XCTAssertEqual(model.cash, "$500.00")
        XCTAssertEqual(model.allocation.map(\.value), ["$7,000.00", "$2,700.00", "$1,382.32"])
        XCTAssertEqual(model.allocation.map(\.name), ["Big Ballers", "Tech Crew", "Underwater"])
    }

    func testThePreviewCashOnlyScenarioHasNoCabalRows() async {
        let model = BalancesModel.preview(.cashOnly)

        await model.load()

        XCTAssertTrue(model.isCashOnly)
        XCTAssertEqual(model.netWorth, "$500.00")
    }

    func testThePreviewFailedScenarioFails() async {
        let model = BalancesModel.preview(.failed)

        await model.load()

        XCTAssertEqual(model.phase, .failed)
    }

    func testThePreviewLoadingScenarioStaysLoading() async {
        let model = BalancesModel.preview(.loading)

        let read = Task { await model.load() }
        for _ in 0..<20 { await Task.yield() }

        XCTAssertEqual(model.phase, .loading)
        read.cancel()
        await read.value
    }

    private func make(portfolio: [Portfolio], cash: [Cash]) throws -> BalancesModel {
        let transport = StubTransport(routes: [
            "/v1/me/portfolio": try portfolio.map { try $0.reply() },
            "/v1/me/balance": try cash.map { try $0.reply() },
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let hints = FakeHintStream()
        return BalancesModel(
            portfolio: PortfolioModel(api: api, hints: hints), balance: BalanceSource(api: api, hints: hints))
    }
}
