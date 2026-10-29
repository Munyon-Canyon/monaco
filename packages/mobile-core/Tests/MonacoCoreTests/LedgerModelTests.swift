import Foundation
import MonacoAPI
import MonacoRanking
import MonacoTestSupport
import XCTest

@MainActor
final class LedgerModelTests: XCTestCase {
    private static func body(_ ledger: Components.Schemas.Ledger) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(ledger), as: UTF8.self)
    }

    private func make(_ replies: [StubTransport.Reply]) -> (LedgerModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (LedgerModel(api: api, week: "2026-W42"), transport)
    }

    private func notFound() throws -> StubTransport.Reply {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 404, code: .ledgerNotFound,
                message: "No Ledger.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
    }

    func testLoadReadsTheWeekOnceAndShowsTheStoredNumbers() async throws {
        let (model, transport) = make([.json(.ok, try Self.body(.week42))])
        XCTAssertEqual(model.state, .idle)

        await model.load()

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen, LedgerScreen(.week42))
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.path, "/v1/me/ledgers/2026-W42")
    }

    func testALedgerNotFoundIsTheNotReadyState() async throws {
        let (model, _) = make([try notFound()])

        await model.load()

        guard case .failed(let error) = model.state else { return XCTFail("not failed") }
        XCTAssertTrue(LedgerCopy.isNotReady(error))
    }

    func testATransportFailureIsRetryableAndALaterLoadRecovers() async throws {
        let (model, _) = make([.failure(URLError(.notConnectedToInternet)), .json(.ok, try Self.body(.loss))])

        await model.load()
        guard case .failed(let error) = model.state else { return XCTFail("not failed") }
        XCTAssertFalse(LedgerCopy.isNotReady(error))

        await model.load()
        XCTAssertEqual(model.state, .loaded(LedgerScreen(.loss)))
    }

    func testALoadedScreenStaysWhileItReloads() async throws {
        let (model, transport) = make([.json(.ok, try Self.body(.week42)), .json(.ok, try Self.body(.noBestCall))])

        await model.load()
        await model.load()

        XCTAssertEqual(model.state, .loaded(LedgerScreen(.noBestCall)))
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
    }

    func testThePreviewModelHoldsTheStateItWasGiven() {
        let model = LedgerModel.preview(state: .loaded(LedgerScreen(.week42)))
        XCTAssertEqual(model.state, .loaded(LedgerScreen(.week42)))
    }
}
