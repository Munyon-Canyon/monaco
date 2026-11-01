import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalRetryTests: XCTestCase {
    private func detailJSON(retryable: Bool) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var detail = Components.Schemas.ProposalDetail.failedSwap(retryable: retryable)
        detail.swap?.swapId = "swap-1"
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }

    private func loadReplies(_ body: String) -> [StubTransport.Reply] {
        [.json(.ok, body), .json(.ok, "{}"), .json(.ok, "{}")]
    }

    private func model(_ transport: StubTransport) -> ProposalDetailModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: ProposalsRepository(api: api), hints: FakeHintStream())
    }

    func testRetryPostsTheSwapWithAnIdempotencyKeyThenReloads() async throws {
        let body = try detailJSON(retryable: true)
        let transport = StubTransport(
            scripted: loadReplies(body) + [
                .json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#)
            ] + loadReplies(body))
        let model = model(transport)
        await model.load()
        XCTAssertEqual(model.retryableSwapID, "swap-1")
        await model.retry()
        XCTAssertTrue(model.didRetry)
        XCTAssertNil(model.errorMessage)
        XCTAssertFalse(model.isRetrying)
        let sent = await transport.sent
        let retry = try XCTUnwrap(sent.first { $0.path == "/v1/swaps/swap-1/retry" })
        XCTAssertEqual(retry.method, .post)
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(retry.headerFields[key])
    }

    func testNoRetryWhenTheSwapIsNotRetryable() async throws {
        let transport = StubTransport(scripted: loadReplies(try detailJSON(retryable: false)))
        let model = model(transport)
        await model.load()
        XCTAssertNil(model.retryableSwapID)
        await model.retry()
        XCTAssertFalse(model.didRetry)
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testSwapNotRetryableShowsTheServerMessage() async throws {
        let body = try detailJSON(retryable: true)
        let problem = """
            {"type":"about:blank","title":"Conflict","status":409,"code":"swap_not_retryable",\
            "message":"This trade can't be retried.","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        let transport = StubTransport(
            scripted: loadReplies(body) + [
                .response(status: .conflict, contentType: "application/problem+json", body: Data(problem.utf8))
            ])
        let model = model(transport)
        await model.load()
        await model.retry()
        XCTAssertFalse(model.didRetry)
        XCTAssertEqual(model.errorMessage, "This trade can't be retried.")
    }

    func testRetryReplaysItsKeyAfterAServerError() async throws {
        let body = try detailJSON(retryable: true)
        let transport = StubTransport(
            scripted: loadReplies(body) + [
                .json(.internalServerError, #"{"message":"Try again"}"#),
                .json(.internalServerError, #"{"message":"Try again"}"#),
            ])
        let model = model(transport)
        await model.load()
        await model.retry()
        await model.retry()
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.compactMap { $0.headerFields[key] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys.first, keys.last)
    }

    private func blockedJSON(statusReason: String = "trade_not_started") throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let detail = Components.Schemas.ProposalDetail.blockedBeforeSwap(statusReason: statusReason)
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }

    private func problem(_ status: HTTPResponse.Status, code: String, message: String) -> StubTransport.Reply {
        let body = """
            {"type":"about:blank","title":"Problem","status":\(status.code),"code":"\(code)",\
            "message":"\(message)","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        return .response(status: status, contentType: "application/problem+json", body: Data(body.utf8))
    }

    func testABlockedTradeWithNoSwapPostsTheProposalRetryThenReloads() async throws {
        let blocked = try blockedJSON()
        let reopened = try detailJSON(retryable: false)
        let transport = StubTransport(
            scripted: loadReplies(blocked) + [.json(.ok, reopened)] + loadReplies(reopened))
        let model = model(transport)
        await model.load()
        XCTAssertTrue(model.retriesUnstartedTrade)
        XCTAssertNil(model.retryableSwapID)
        await model.retry()
        XCTAssertTrue(model.didRetry)
        XCTAssertNil(model.errorMessage)
        XCTAssertFalse(model.isRetrying)
        XCTAssertFalse(model.retriesUnstartedTrade)
        let sent = await transport.sent
        let retry = try XCTUnwrap(sent.first { $0.path == "/v1/proposals/proposal-1/retry-trade" })
        XCTAssertEqual(retry.method, .post)
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(retry.headerFields[key])
    }

    func testOnlyTradeNotStartedOffersTheProposalRetry() async throws {
        for code in ["no_route", "swap_failed", "pot_exceeded"] {
            let transport = StubTransport(scripted: loadReplies(try blockedJSON(statusReason: code)))
            let model = model(transport)
            await model.load()
            XCTAssertFalse(model.retriesUnstartedTrade, code)
            await model.retry()
            XCTAssertFalse(model.didRetry, code)
            let count = await transport.sent.count
            XCTAssertEqual(count, 3, code)
        }
    }

    func testAProposalRetryTheServerRefusesShowsItsMessage() async throws {
        let refusals: [(HTTPResponse.Status, String, String)] = [
            (.conflict, "swap_not_retryable", "This trade can't be retried."),
            (.forbidden, "not_cabal_member", "You are not a member of this cabal."),
        ]
        for (status, code, message) in refusals {
            let transport = StubTransport(
                scripted: loadReplies(try blockedJSON()) + [problem(status, code: code, message: message)])
            let model = model(transport)
            await model.load()
            await model.retry()
            XCTAssertFalse(model.didRetry, code)
            XCTAssertEqual(model.errorMessage, message, code)
            XCTAssertTrue(model.retriesUnstartedTrade, code)
        }
    }

    func testAProposalRetryReplaysItsKeyAfterAServerError() async throws {
        let transport = StubTransport(
            scripted: loadReplies(try blockedJSON()) + [
                .json(.internalServerError, #"{"message":"Try again"}"#),
                .json(.internalServerError, #"{"message":"Try again"}"#),
            ])
        let model = model(transport)
        await model.load()
        await model.retry()
        await model.retry()
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.compactMap { $0.headerFields[key] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys.first, keys.last)
    }
}
