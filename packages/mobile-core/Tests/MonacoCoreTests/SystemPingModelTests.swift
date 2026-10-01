import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class SystemPingModelTests: XCTestCase {
    private let pingID = "01890a5d-ac96-774b-bcce-b302099a8057"

    func testSendPostsOnceAndReusesTheIdempotencyKeyAfterATransportError() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.cannotConnectToHost)),
            created(note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: false),
        ])
        let model = makeModel(transport)

        await model.send(note: "hi")
        let failed = model.state
        guard case .failed(.transport(let error)) = failed else {
            XCTFail("expected a transport failure, got \(failed)")
            return
        }
        XCTAssertEqual(error.code, .cannotConnectToHost)

        await model.send(note: "hi")

        let sent = await transport.sent
        let posts = sent.filter { $0.path == "/v1/system/pings" }
        let keyHeaderName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = posts.map { $0.headerFields[keyHeaderName] }
        XCTAssertEqual(posts.count, 2, sent.map(\.path).description)
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
        let loaded = model.state
        guard case .loaded(let ping) = loaded else {
            XCTFail("expected the retried ping, got \(loaded)")
            return
        }
        XCTAssertEqual(ping.id, pingID)
        XCTAssertFalse(ping.echoed)
    }

    func testPingEchoedHintTriggersOneGet() async throws {
        let transport = StubTransport(scripted: [
            created(note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: true),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.send(note: "hi")
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        let before = await getCount(transport)

        await hints.send(.changed(.user("user-1"), what: "ping_echoed", id: "1"))
        let arrived = await waitUntil {
            guard case .loaded(let ping) = model.state else { return false }
            return ping.echoed
        }
        task.cancel()

        let gets = await getCount(transport)
        XCTAssertTrue(arrived)
        XCTAssertEqual(gets, before + 1)
    }

    func testResyncTriggersOneGet() async throws {
        let (model, transport, hints) = try await observingAfterSend()
        let before = await getCount(transport)

        await hints.send(.resync)
        let arrived = await waitUntil { await self.getCount(transport) == before + 1 }

        let gets = await getCount(transport)
        XCTAssertTrue(arrived)
        XCTAssertEqual(gets, before + 1)
        guard case .loaded = model.state else {
            XCTFail("expected the ping to stay loaded")
            return
        }
    }

    func testHintForAnotherKeyTriggersNoGet() async throws {
        let (_, transport, hints) = try await observingAfterSend()
        let before = await getCount(transport)

        await hints.send(.changed(.cabal("cabal-1"), what: "ping_echoed", id: "9"))
        await hints.send(.changed(.user("user-1"), what: "cabal_changed", id: "8"))
        await hints.send(.resync)
        let settled = await waitUntilQuiet(transport, atLeast: before + 1)

        XCTAssertEqual(settled, before + 1)
    }

    func testAStaleGetDoesNotOverwriteANewerPing() async throws {
        let transport = StubTransport(scripted: [
            created(note: "hi", echoed: false),
            .gate,
            ping(status: .ok, note: "hi", echoed: true),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        let observe = Task { await model.observe() }
        let send = Task { await model.send(note: "hi") }
        let parked = await waitUntil { await transport.sent.count >= 2 }
        _ = await waitUntil { await hints.subscriberCount > 0 }

        await hints.send(.changed(.user("user-1"), what: "ping_echoed", id: "1"))
        let echoed = await waitUntil {
            guard case .loaded(let ping) = model.state else { return false }
            return ping.echoed
        }
        await transport.releaseGate(ping(status: .ok, note: "hi", echoed: false))
        await send.value
        observe.cancel()

        XCTAssertTrue(parked)
        XCTAssertTrue(echoed)
        let loaded = model.state
        guard case .loaded(let ping) = loaded else {
            XCTFail("expected the newer ping, got \(loaded)")
            return
        }
        XCTAssertTrue(ping.echoed)
    }

    func testConcurrentSendsPostOnce() async throws {
        let transport = StubTransport(scripted: [
            created(note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: false),
        ])
        let model = makeModel(transport)

        async let first: Void = model.send(note: "hi")
        async let second: Void = model.send(note: "hi")
        await first
        await second

        let sent = await transport.sent
        let posts = sent.filter { $0.path == "/v1/system/pings" }
        XCTAssertEqual(posts.count, 1, sent.map(\.path).description)
        XCTAssertFalse(model.isSending)
    }

    func testProblemResponseFailsAndToastCopyUsesTheServerMessage() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank,
                title: "Error",
                status: 404,
                code: .notFound,
                message: "No such ping.",
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: false
            ))
        let model = makeModel(transport)

        await model.send(note: "hi")

        let failed = model.state
        guard case .failed(.problem(let problem)) = failed else {
            XCTFail("expected a problem, got \(failed)")
            return
        }
        XCTAssertEqual(problem.message, "No such ping.")
        XCTAssertEqual(ToastCopy.message(for: .problem(problem)), "No such ping.")
    }

    func testRefreshKeepsTheLoadedPingUntilTheNextArrives() async throws {
        let transport = StubTransport(scripted: [
            created(note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: false),
            .hang,
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.send(note: "hi")
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        let before = await transport.sent.count

        await hints.send(.resync)
        let started = await waitUntil { await transport.sent.count == before + 1 }
        let during = model.state
        task.cancel()

        XCTAssertTrue(started)
        guard case .loaded(let ping) = during else {
            XCTFail("expected the previous ping to stay, got \(during)")
            return
        }
        XCTAssertEqual(ping.note, "hi")
        XCTAssertFalse(ping.echoed)
    }

    #if DEBUG
    func testPreviewModelEchoesAPingWithoutTheNetwork() async {
        let model = SystemPingModel.preview()

        await model.observe()
        await model.send(note: "hi")

        let state = model.state
        guard case .loaded(let ping) = state else {
            XCTFail("expected the preview to echo a ping, got \(state)")
            return
        }
        XCTAssertTrue(ping.echoed)
        XCTAssertEqual(ping.note, "hi")
    }
    #endif

    private func created(note: String, echoed: Bool) -> StubTransport.Reply {
        .json(.created, body(note: note, echoed: echoed))
    }

    private func ping(status: HTTPResponse.Status, note: String, echoed: Bool) -> StubTransport.Reply {
        .json(status, body(note: note, echoed: echoed))
    }

    private func body(note: String, echoed: Bool) -> String {
        #"{"id":"\#(pingID)","note":"\#(note)","echoed":\#(echoed)}"#
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> SystemPingModel {
        SystemPingModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
    }

    private func observingAfterSend() async throws -> (SystemPingModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: [
            created(note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: false),
            ping(status: .ok, note: "hi", echoed: true),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.send(note: "hi")
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        addTeardownBlock { task.cancel() }
        return (model, transport, hints)
    }

    private func getCount(_ transport: StubTransport) async -> Int {
        await transport.sent.filter { ($0.path ?? "").hasPrefix("/v1/system/pings/") }.count
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private func waitUntilQuiet(_ transport: StubTransport, atLeast minimum: Int) async -> Int {
        var last = await getCount(transport)
        var quiet = 0
        for _ in 0..<500 {
            if last >= minimum {
                quiet += 1
                if quiet == 30 { return last }
            }
            await Task.yield()
            let now = await getCount(transport)
            if now != last {
                quiet = 0
                last = now
            }
        }
        return await getCount(transport)
    }
}
