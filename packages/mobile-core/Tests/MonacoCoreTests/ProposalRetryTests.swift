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
}
