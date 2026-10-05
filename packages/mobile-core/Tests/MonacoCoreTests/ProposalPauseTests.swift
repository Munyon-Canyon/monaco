import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalPauseTests: XCTestCase {
    private let pausedBody =
        #"{"slice_micros":"1","share_units":"1","min_micros":"1","pause":{"reasons":["ops"],"since":"2026-01-01T00:00:00Z"}}"#
    private let openBody = #"{"slice_micros":"1","share_units":"1","min_micros":"1","pause":null}"#

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    @MainActor
    private func waitUntil(_ condition: @escaping @MainActor () async -> Bool) async -> Bool {
        for _ in 0..<100 {
            if await condition() { return true }
            await Task.yield()
        }
        return await condition()
    }

    func testRepositoryReadsThePauseFromTheCashOutPreview() async throws {
        let transport = StubTransport(scripted: [.json(.ok, pausedBody), .json(.ok, openBody)])
        let repository = repository(transport)
        let paused = try await repository.isPaused(cabalID: "c")
        let running = try await repository.isPaused(cabalID: "c")
        XCTAssertTrue(paused)
        XCTAssertFalse(running)
        let path = await transport.sent.first?.path
        XCTAssertEqual(path, "/v1/cabals/c/cashouts/preview")
    }
}

final class ProposalPauseModelTests: XCTestCase {
    private let pausedBody =
        #"{"slice_micros":"1","share_units":"1","min_micros":"1","pause":{"reasons":["ops"],"since":"2026-01-01T00:00:00Z"}}"#
    private let openBody = #"{"slice_micros":"1","share_units":"1","min_micros":"1","pause":null}"#

    @MainActor
    private func model(_ transport: StubTransport, hints: FakeHintStream) -> ProposalPauseModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalPauseModel(cabalID: "c", repository: ProposalsRepository(api: api), hints: hints)
    }

    @MainActor
    func testLoadSetsPausedAndAFailureKeepsTheLastValue() async {
        let transport = StubTransport(scripted: [.json(.ok, pausedBody), .failure(URLError(.notConnectedToInternet))])
        let model = model(transport, hints: FakeHintStream())
        await model.load()
        XCTAssertTrue(model.isPaused)
        await model.load()
        XCTAssertTrue(model.isPaused)
    }

    @MainActor
    func testAPauseChangedHintRereadsThePause() async {
        let transport = StubTransport(scripted: [.json(.ok, pausedBody), .json(.ok, openBody)])
        let hints = FakeHintStream()
        let model = model(transport, hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        for _ in 0..<100 where await hints.subscriberCount != 1 { await Task.yield() }
        model.setVisible(true)
        await hints.send(.changed(.cabal("c"), what: "pause_changed", id: "1"))
        for _ in 0..<200 where model.isPaused { await Task.yield() }
        XCTAssertFalse(model.isPaused)
    }
}

final class PendingVotesPauseTests: XCTestCase {
    @MainActor
    func testPendingVotesMarkPausedCabals() async {
        let vote =
            #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
        let paused =
            #"{"slice_micros":"1","share_units":"1","min_micros":"1","pause":{"reasons":["ops"],"since":"2026-01-01T00:00:00Z"}}"#
        let transport = StubTransport(scripted: [
            .json(.ok, vote), .failure(URLError(.badServerResponse)), .json(.ok, paused),
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = PendingVotesModel(repository: ProposalsRepository(api: api), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.pausedCabals, ["c"])
    }
}
