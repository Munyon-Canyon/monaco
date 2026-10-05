import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class PeopleSearchModelTests: XCTestCase {
    private let samples = Components.Schemas.UserSummary.samples

    func testTypingWithinTheDebounceSendsOneRequestForTheLastQuery() async throws {
        let transport = StubTransport(.json(.ok, try Self.result(samples)))
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = "m"
        model.query = "ma"
        model.query = "may"
        XCTAssertEqual(model.state, .loading)
        let parked = await clock.state.until { $0.pending == 1 }
        XCTAssertTrue(parked)
        clock.advance(by: PeopleSearchModel.debounce)
        let loaded = await waitUntil { model.state == .loaded(self.samples) }
        XCTAssertTrue(loaded)
        let sent = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(sent, ["/v1/users?query=may"])
    }

    func testAnAtAndOneCharacterSendsNothing() async {
        let transport = StubTransport(scripted: [])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = " @a "
        XCTAssertEqual(model.normalized, "a")
        XCTAssertEqual(model.state, .idle)
        clock.advance(by: PeopleSearchModel.debounce)
        await Task.yield()
        XCTAssertEqual(clock.state.current.pending, 0)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testTheLeadingAtIsStripped() async throws {
        let transport = StubTransport(.json(.ok, try Self.result([samples[2]])))
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = "@qa"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        let loaded = await waitUntil { model.state == .loaded([self.samples[2]]) }
        XCTAssertTrue(loaded)
        let sent = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(sent, ["/v1/users?query=qa"])
    }

    func testALateResponseForAnOlderQueryIsDropped() async throws {
        let transport = StubTransport(scripted: [.gate, .json(.ok, try Self.result([samples[0]]))])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = "ma"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        await transport.waitForRequest()
        model.query = "may"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        let loaded = await waitUntil { model.state == .loaded([self.samples[0]]) }
        XCTAssertTrue(loaded)
        await transport.releaseGate(.json(.ok, try Self.result(samples)))
        for _ in 0..<20 { await Task.yield() }
        XCTAssertEqual(model.state, .loaded([samples[0]]))
        XCTAssertNil(model.toast)
        let sent = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(sent, ["/v1/users?query=ma", "/v1/users?query=may"])
    }

    func testRateLimitAndServerErrorsLandInTheErrorStateWithToastCopy() async {
        for (status, code, message) in [(429, "rate_limited", "Slow down."), (500, "internal", "Something broke.")] {
            let clock = TestClock()
            let model = makeModel(StubTransport(Self.problem(status, code, message)), clock: clock)
            model.query = "may"
            _ = await clock.state.until { $0.pending == 1 }
            clock.advance(by: PeopleSearchModel.debounce)
            let failed = await waitUntil {
                if case .failed = model.state { return true }
                return false
            }
            XCTAssertTrue(failed, "\(status)")
            guard case .failed(let error) = model.state else { continue }
            XCTAssertEqual(ToastCopy.message(for: error), message)
            XCTAssertNil(model.toast)
        }
    }

    func testAFailureOverResultsKeepsThemAndToasts() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.result(samples)), .failure(URLError(.notConnectedToInternet)),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = "ma"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        _ = await waitUntil { model.state == .loaded(self.samples) }
        model.retry()
        let toasted = await waitUntil { model.toast != nil }
        XCTAssertTrue(toasted)
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
        XCTAssertEqual(model.state, .loaded(samples))
    }

    func testRetryRerunsTheQueryWithoutWaiting() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)), .json(.ok, try Self.result([])),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        model.query = "zzq"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        _ = await waitUntil {
            if case .failed = model.state { return true }
            return false
        }
        model.retry()
        let empty = await waitUntil { model.state == .loaded([]) }
        XCTAssertTrue(empty)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testClearingTheFieldGoesIdle() async throws {
        let clock = TestClock()
        let model = makeModel(StubTransport(.json(.ok, try Self.result(samples))), clock: clock)
        model.query = "ma"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: PeopleSearchModel.debounce)
        _ = await waitUntil { model.state == .loaded(self.samples) }
        model.query = ""
        XCTAssertEqual(model.state, .idle)
        XCTAssertFalse(model.isSearching)
    }

    private func makeModel(_ transport: StubTransport, clock: TestClock) -> PeopleSearchModel {
        PeopleSearchModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            clock: clock
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func result(_ users: [Components.Schemas.UserSummary]) throws -> String {
        String(decoding: try JSONEncoder().encode(Components.Schemas.UserSearchResult(users: users)), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}
