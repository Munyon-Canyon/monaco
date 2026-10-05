import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CashOutModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private var previewPath: String { "/v1/cabals/\(cabalID)/cashouts/preview" }
    private var cashOutsPath: String { "/v1/cabals/\(cabalID)/cashouts" }

    func testLoadReadsThePreview() async throws {
        let transport = StubTransport(scripted: [try Self.json(.ok, Components.Schemas.CashOutPreview.sample)])
        let model = makeModel(transport)

        await model.load()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [previewPath])
        XCTAssertEqual(
            model.preview,
            CashOutPreview(sliceMicros: 200_150_000, shareUnits: 200_150_000, minMicros: 100_000, pause: nil))
    }

    func testAPausedPreviewCarriesItsCause() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sampleDepositPause)
        ])
        let model = makeModel(transport)

        await model.load()

        XCTAssertEqual(model.preview?.pause, CabalPause(cause: .externalDeposit))
    }

    func testADustStrandingAmountSendsAllTrue() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try Self.json(.accepted, Components.Schemas.CashOutJob.sample(status: .started, payoutMicros: "200150000")),
        ])
        let model = makeModel(transport)
        await model.load()

        let result = await model.submit(enteredMicros: 200_100_000)

        guard case .started(let job) = result else {
            return XCTFail("expected a job, got \(String(describing: result))")
        }
        XCTAssertEqual(job.startedToast, "Cashing out $200.15. It lands in your balance in about a minute")
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.method, .post)
        XCTAssertEqual(sent.last?.path, cashOutsPath)
        XCTAssertNotNil(sent.last?.headerFields[try idempotencyKey()])
        let body = try await lastBody(transport)
        XCTAssertEqual(body, ["all": true])
    }

    func testAPartialAmountSendsUSDCMicros() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try Self.json(.accepted, Components.Schemas.CashOutJob.sample(status: .started)),
        ])
        let model = makeModel(transport)
        await model.load()

        _ = await model.submit(enteredMicros: 1_000_000)

        let body = try await lastBody(transport)
        XCTAssertEqual(body, ["usdc_micros": "1000000"])
    }

    func testARetryAfterATransportErrorReusesTheKey() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            .failure(URLError(.networkConnectionLost)),
            try Self.json(.accepted, Components.Schemas.CashOutJob.sample(status: .started)),
        ])
        let model = makeModel(transport)
        await model.load()

        let first = await model.submit(enteredMicros: 1_000_000)
        _ = await model.submit(enteredMicros: 1_000_000)

        XCTAssertEqual(first, .refused("You're offline. Try again."))
        let keys = try await transport.sent.dropFirst().map { $0.headerFields[try idempotencyKey()] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testCabalPausedRefusalShowsThePauseCopyAndReloads() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try problem(.cabalPaused, status: 409, "This cabal is paused."),
            try Self.json(.ok, Components.Schemas.CashOutPreview.sampleOpsPause),
        ])
        let model = makeModel(transport)
        await model.load()

        let result = await model.submit(enteredMicros: 1_000_000)

        XCTAssertEqual(result, .refused(CabalPause(cause: .ops).message))
        XCTAssertEqual(model.preview?.pause, CabalPause(cause: .ops))
    }

    func testCashOutInProgressHasItsOwnLine() async throws {
        let result = try await refusal(.cashOutInProgress, "busy")
        XCTAssertEqual(result, .refused("A cash out is already running for this cabal."))
    }

    func testPriceUnavailableShowsTheServerMessage() async throws {
        let result = try await refusal(.priceUnavailable, "Prices are catching up. Try again in a moment.")
        XCTAssertEqual(result, .refused("Prices are catching up. Try again in a moment."))
    }

    func testAPausedPreviewSubmitsNothing() async throws {
        let transport = StubTransport(scripted: [try Self.json(.ok, Components.Schemas.CashOutPreview.sampleOpsPause)])
        let model = makeModel(transport)
        await model.load()

        let result = await model.submit(enteredMicros: 1_000_000)

        XCTAssertNil(result)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testPauseChangedOnThisCabalRefetchesAndOtherHintsDoNot() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try Self.json(.ok, Components.Schemas.CashOutPreview.sampleOpsPause),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8061"), what: "pause_changed", id: "1"))
        await hints.send(.changed(.cabal(cabalID), what: "members", id: "2"))
        await hints.send(.changed(.cabal(cabalID), what: "pause_changed", id: "3"))

        let paused = await waitUntil { model.preview?.pause != nil }
        XCTAssertTrue(paused)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAResyncRefetchesThePreview() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try Self.json(.ok, Components.Schemas.CashOutPreview.sampleNoStake),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.resync)

        let refetched = await waitUntil { model.preview?.hasStake == false }
        XCTAssertTrue(refetched)
    }

    func testAFailedFirstLoadFails() async throws {
        let model = makeModel(StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))]))

        await model.load()

        guard case .failed = model.state else { return XCTFail("expected a failure, got \(model.state)") }
    }

    private func refusal(_ code: Components.Schemas.ErrorCode, _ message: String) async throws -> CashOutSubmitResult? {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            try problem(code, status: 409, message),
        ])
        let model = makeModel(transport)
        await model.load()
        return await model.submit(enteredMicros: 1_000_000)
    }

    private func lastBody(_ transport: StubTransport) async throws -> NSDictionary {
        let bodies = await transport.sentBodies
        let data = try XCTUnwrap(bodies.last ?? nil)
        return try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? NSDictionary)
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CashOutModel {
        CashOutModel(
            cabalID: cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
    }

    static func json(_ status: HTTPResponse.Status, _ value: some Encodable) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(status, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func problem(_ code: Components.Schemas.ErrorCode, status: Int, _ message: String) throws
        -> StubTransport.Reply
    {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: status, code: code, message: message,
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false
            ))
    }

    private func idempotencyKey() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
