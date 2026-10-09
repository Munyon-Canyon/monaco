import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalPriceMoveTests: XCTestCase {
    func testBuyPriceRisesWhenFewerSharesComeBack() throws {
        let move = try XCTUnwrap(ProposalPriceMove(votedQuote: 100_000_000, currentQuote: 95_000_000, isSell: false))
        XCTAssertEqual(move.basisPoints, 526)
        XCTAssertEqual(move.percentText, "5.3%")
    }

    func testSellPriceFallsWhenLessUsdcComesBack() throws {
        let move = try XCTUnwrap(ProposalPriceMove(votedQuote: 20_000_000, currentQuote: 19_000_000, isSell: true))
        XCTAssertEqual(move.basisPoints, 500)
        XCTAssertEqual(move.percentText, "5.0%")
    }

    func testPercentIsAbsoluteAndRoundsToTenths() throws {
        let cheaper = try XCTUnwrap(ProposalPriceMove(votedQuote: 100, currentQuote: 110, isSell: false))
        XCTAssertEqual(cheaper.basisPoints, 909)
        XCTAssertEqual(cheaper.percentText, "9.1%")
    }

    func testLargeAtomicsDoNotOverflow() throws {
        let move = try XCTUnwrap(
            ProposalPriceMove(votedQuote: Int64.max / 2, currentQuote: Int64.max / 4, isSell: false))
        XCTAssertEqual(move.basisPoints, 10_000)
    }

    func testNoMoveWithoutAPositiveQuote() {
        XCTAssertNil(ProposalPriceMove(votedQuote: 0, currentQuote: 5, isSell: false))
        XCTAssertNil(ProposalPriceMove(votedQuote: 5, currentQuote: 0, isSell: false))
    }

    func testStepperNoteNamesThePriceMove() {
        let stepper = ProposalStepper.make(
            status: .passed, isSell: false, swapFailed: true, expiresAt: .now, failureMessage: "ignored",
            priceMoved: true)
        XCTAssertEqual(
            stepper.steps[1].note,
            "The price moved past the cabal's limit since the vote. The money is still in the pot.")
    }
}

@MainActor
final class ProposalPriceMovedModelTests: XCTestCase {
    private func detailJSON(code: String) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var detail = Components.Schemas.ProposalDetail.failedSwap(retryable: true, failureCode: code)
        detail.swap?.swapId = "swap-1"
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }

    private func model(_ transport: StubTransport) -> ProposalDetailModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: ProposalsRepository(api: api), hints: FakeHintStream())
    }

    private func preview(_ quote: Int64) -> String {
        #"{"quote_out_amount":\#(quote),"advisory_code":null,"advisory_message":null,"pot_value_micros":100000000}"#
    }

    func testFailureCodeParsesFromTheSwap() throws {
        let moved = ProposalDetail(.priceMoved).summary.swap
        XCTAssertEqual(moved?.failureCode, "price_moved")
        XCTAssertEqual(moved?.isPriceMoved, true)
        XCTAssertEqual(ProposalDetail(.failedSwap(retryable: true)).summary.swap?.isPriceMoved, false)
    }

    func testPriceMovedLoadsTheLiveQuoteAndRetriesAtTheCurrentPrice() async throws {
        let body = try detailJSON(code: "price_moved")
        let transport = StubTransport(scripted: [
            .json(.ok, body), .json(.ok, "{}"), .json(.ok, "{}"), .json(.ok, preview(13_600_000)),
            .json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#),
            .json(.ok, body), .json(.ok, "{}"), .json(.ok, "{}"), .json(.ok, preview(13_600_000)),
        ])
        let model = model(transport)
        await model.load()
        XCTAssertEqual(model.priceMove?.votedQuote, 14_250_000)
        XCTAssertEqual(model.priceMove?.currentQuote, 13_600_000)
        XCTAssertTrue(model.retriesAtCurrentPrice)
        let allSent = await transport.sent
        let previewPath = try XCTUnwrap(allSent.compactMap(\.path).first { $0.contains("/proposals/preview") })
        XCTAssertTrue(previewPath.contains("symbol=GOOGLx"))
        XCTAssertTrue(previewPath.contains("usdc_micros=25000000"))
        await model.retry()
        XCTAssertTrue(model.didRetry)
        let bodies = await transport.sentBodies.compactMap { $0 }.map { String(decoding: $0, as: UTF8.self) }
        XCTAssertEqual(bodies.count, 1)
        let sentBody = try JSONSerialization.jsonObject(with: Data(bodies[0].utf8)) as? [String: Bool]
        XCTAssertEqual(sentBody, ["at_current_price": true])
    }

    func testOtherFailuresRetryWithNoBodyAndLoadNoQuote() async throws {
        let body = try detailJSON(code: "jupiter_failed")
        let transport = StubTransport(scripted: [
            .json(.ok, body), .json(.ok, "{}"), .json(.ok, "{}"),
            .json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#),
            .json(.ok, body), .json(.ok, "{}"), .json(.ok, "{}"),
        ])
        let model = model(transport)
        await model.load()
        XCTAssertNil(model.priceMove)
        XCTAssertFalse(model.retriesAtCurrentPrice)
        await model.retry()
        let sent = await transport.sent
        XCTAssertFalse(sent.compactMap(\.path).contains { $0.contains("/proposals/preview") })
        XCTAssertNotNil(sent.first { $0.path == "/v1/swaps/swap-1/retry" })
        let bodies = await transport.sentBodies
        XCTAssertTrue(bodies.compactMap { $0 }.isEmpty)
    }

    func testNoLiveQuoteHidesTheMoveButStillRetriesAtCurrentPrice() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try detailJSON(code: "price_moved")), .json(.ok, "{}"), .json(.ok, "{}"),
            .json(.internalServerError, #"{"message":"down"}"#),
        ])
        let model = model(transport)
        await model.load()
        XCTAssertNil(model.priceMove)
        XCTAssertTrue(model.retriesAtCurrentPrice)
        XCTAssertNil(model.errorMessage)
    }
}
