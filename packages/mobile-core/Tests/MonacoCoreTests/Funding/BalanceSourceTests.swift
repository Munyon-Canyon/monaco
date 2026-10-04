import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class BalanceSourceTests: XCTestCase {
    private let userID = "01890a5d-ac96-774b-bcce-b302099a8058"

    func testLoadSendsOneGetAndParsesEveryField() async throws {
        let transport = StubTransport(
            .json(.ok, Self.body(available: "22500000", onChain: "25000000", inFlight: "2500000", address: "wallet-7")))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/balance"])
        let balance = try XCTUnwrap(model.balance)
        XCTAssertEqual(balance.availableMicros, 22_500_000)
        XCTAssertEqual(balance.onChainMicros, 25_000_000)
        XCTAssertEqual(balance.inFlightMicros, 2_500_000)
        XCTAssertEqual(balance.depositAddress, "wallet-7")
        XCTAssertEqual(balance.asOf, Date(timeIntervalSince1970: 1_759_579_200))
    }

    func testTheSampleParses() throws {
        let balance = try AccountBalance(.sample)

        XCTAssertEqual(balance.availableMicros, 248_500_000)
        XCTAssertEqual(balance.inFlightMicros, 50_000_000)
    }

    func testABalanceChangedHintRefetches() async throws {
        try await assertRefetch(after: .changed(.user(userID), what: "balance_changed", id: "1"))
    }

    func testAResyncRefetches() async throws {
        try await assertRefetch(after: .resync)
    }

    func testOtherHintsSendNothing() async throws {
        let transport = StubTransport(.json(.ok, Self.body(available: "12000000")))
        let hints = FakeHintStream()
        let model = BalanceSource(api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user(userID), what: "profile_changed", id: "1"))
        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8060"), what: "balance_changed", id: "2"))
        await hints.send(.changed(.global, what: "balance_changed", id: "3"))
        await hints.send(.resync)
        let marker = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(marker)
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAHiddenScreenWaitsToRefetchUntilItIsVisible() async throws {
        let transport = StubTransport(.json(.ok, Self.body(available: "12000000")))
        let hints = FakeHintStream()
        let model = BalanceSource(api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        model.setVisible(false)
        await hints.send(.changed(.user(userID), what: "balance_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)

        model.setVisible(true)
        let refetched = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refetched)
    }

    func testAHintRefetchThatFailsKeepsTheBalanceWithoutAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.body(available: "12000000")),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, Self.body(available: "17000000")),
        ])
        let hints = FakeHintStream()
        let model = BalanceSource(api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.resync)
        await hints.send(.changed(.user(userID), what: "balance_changed", id: "1"))
        let refetched = await waitUntil { model.balance?.availableMicros == 17_000_000 }

        XCTAssertTrue(refetched)
        XCTAssertEqual(model.failureTick, 0)
        XCTAssertNil(model.lastError)
    }

    func testAHintRefetchAfterAFailedReadDoesNotToastAgain() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, Self.body(available: "12000000")),
        ])
        let hints = FakeHintStream()
        let model = BalanceSource(api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.resync)
        await hints.send(.resync)
        let recovered = await waitUntil { model.balance?.availableMicros == 12_000_000 }

        XCTAssertTrue(recovered)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testAFirstReadThatFailsLeavesNothingOnScreen() async throws {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()

        guard case .failed(let error) = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertNil(model.balance)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(BalanceSource.message(for: error), "You're offline. Try again.")
    }

    func testAFailedRefreshKeepsTheBalanceOnScreen() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.body(available: "17000000")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()
        await model.load()

        XCTAssertEqual(model.balance?.availableMicros, 17_000_000)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(BalanceSource.message(for:)), "You're offline. Try again.")
    }

    func testRPCUnavailableAsksForAPullToRefresh() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 503, code: .rpcUnavailable,
                message: "Solana RPC is unavailable.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true
            ))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()

        let error = try XCTUnwrap(model.lastError)
        XCTAssertEqual(BalanceSource.message(for: error), "Balance is temporarily unavailable. Pull to refresh.")
    }

    func testAnyOtherProblemShowsTheServerMessage() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 404, code: .userNotFound,
                message: "That account doesn't exist.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false
            ))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()

        let error = try XCTUnwrap(model.lastError)
        XCTAssertEqual(BalanceSource.message(for: error), "That account doesn't exist.")
    }

    func testAMalformedAmountFailsTheRead() async throws {
        let transport = StubTransport(.json(.ok, Self.body(available: "12.5")))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        await model.load()

        guard case .failed(.decoding) = model.state else {
            XCTFail("expected a decoding failure, got \(model.state)")
            return
        }
    }

    func testACancelledReadIsNotAFailure() async {
        let transport = StubTransport(.hang)
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        let reading = Task { await model.load() }
        await transport.waitForRequest()
        reading.cancel()
        await reading.value

        XCTAssertEqual(model.state, .loading)
        XCTAssertEqual(model.failureTick, 0)
        XCTAssertNil(model.lastError)
    }

    func testASlowReadNeverOverwritesANewerOne() async {
        let transport = StubTransport(scripted: [.gate, .json(.ok, Self.body(available: "17000000"))])
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        let slow = Task { await model.load() }
        await transport.waitForRequest()
        await model.load()
        await transport.releaseGate(.json(.ok, Self.body(available: "12000000")))
        await slow.value

        XCTAssertEqual(model.balance?.availableMicros, 17_000_000)
    }

    func testAmountsParseFromDigitStrings() throws {
        XCTAssertEqual(try AccountBalance.micros("0"), 0)
        XCTAssertEqual(try AccountBalance.micros("1500000"), 1_500_000)
        XCTAssertEqual(try AccountBalance.micros("9223372036854775807"), .max)
    }

    func testAmountsThatAreNotPlainDigitsAreRejected() {
        for wire in ["", "-1", "+1", "1.5", " 1", "1e6", "9223372036854775808"] {
            XCTAssertThrowsError(try AccountBalance.micros(wire), wire) { error in
                guard case APIError.decoding = error else {
                    XCTFail("\(wire) threw \(error), not a decoding error")
                    return
                }
            }
        }
    }

    private func assertRefetch(after hint: Hint, file: StaticString = #filePath, line: UInt = #line) async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.body(available: "12000000")),
            .json(.ok, Self.body(available: "17000000")),
        ])
        let hints = FakeHintStream()
        let model = BalanceSource(api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed, file: file, line: line)

        await hints.send(hint)
        let refetched = await waitUntil { model.balance?.availableMicros == 17_000_000 }

        XCTAssertTrue(refetched, file: file, line: line)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/balance", "/v1/me/balance"], file: file, line: line)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    static func body(
        available: String, onChain: String? = nil, inFlight: String = "0", address: String = "wallet-1"
    ) -> String {
        #"{"available_micros":"\#(available)","on_chain_micros":"\#(onChain ?? available)","#
            + #""in_flight_micros":"\#(inFlight)","deposit_address":"\#(address)","as_of":"2025-10-04T12:00:00Z"}"#
    }
}
