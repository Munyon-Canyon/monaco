import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class PortfolioModelTests: XCTestCase {
    func testLoadReadsThePortfolioOnce() async throws {
        let (model, transport, _) = try make([.portfolio(.sample)])

        await model.load()

        XCTAssertEqual(model.summary, PortfolioSummary(.sample))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/portfolio"])
    }

    func testAFailedFirstLoadIsTheErrorStateWithNoToast() async throws {
        let (model, _, _) = try make([.failure])

        await model.load()

        guard case .failed(.transport) = model.state else { return XCTFail("want a transport failure") }
        XCTAssertNil(model.toast)
    }

    func testAFailedRefreshKeepsTheContentAndToasts() async throws {
        let (model, _, _) = try make([.portfolio(.sample), .failure])
        await model.load()

        await model.load()

        XCTAssertEqual(model.summary, PortfolioSummary(.sample))
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testAnEmptyPortfolioIsLoadedAndEmpty() async throws {
        let (model, _, _) = try make([.portfolio(.sampleEmpty)])

        await model.load()

        XCTAssertEqual(model.summary?.isEmpty, true)
    }

    func testAUserHintAndALeaderboardsHintEachReadAgain() async throws {
        let (model, transport, hints) = try make([.portfolio(.sample), .portfolio(.sample), .portfolio(.sampleLoss)])
        await model.load()
        let task = Task { await model.observe() }
        while await hints.subscriberCount < 2 { await Task.yield() }

        await hints.send(.changed(.user("u1"), what: "balance", id: "1"))
        await transport.waitForRequests(2)
        await hints.send(.changed(.global, what: "leaderboards_updated", id: "2"))
        await transport.waitForRequests(3)
        while model.summary != PortfolioSummary(.sampleLoss) { await Task.yield() }

        XCTAssertEqual(model.summary, PortfolioSummary(.sampleLoss))
        task.cancel()
    }

    private enum Script {
        case portfolio(Components.Schemas.MyPortfolio)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .portfolio(let portfolio):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(portfolio), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(_ script: [Script]) throws -> (PortfolioModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (PortfolioModel(api: api, hints: hints), transport, hints)
    }
}
