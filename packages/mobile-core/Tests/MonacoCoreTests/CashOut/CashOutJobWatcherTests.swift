import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CashOutJobWatcherTests: XCTestCase {
    private let started = CashOutJob(
        id: "01890a5d-ac96-774b-bcce-b302099a8070", cabalID: "01890a5d-ac96-774b-bcce-b302099a8059",
        status: .started, payoutMicros: 1_000_000, resultCode: nil)
    private var jobPath: String { "/v1/cabals/\(started.cabalID)/cashouts/\(started.id)" }

    func testARunningJobIsWatchedUntilACashoutChangedHintEndsIt() async throws {
        let transport = StubTransport(scripted: [
            try CashOutModelTests.json(.ok, Components.Schemas.CashOutJob.sample(status: .selling)),
            try CashOutModelTests.json(.ok, Components.Schemas.CashOutJob.sample(status: .completed)),
        ])
        let hints = FakeHintStream()
        let watcher = makeWatcher(transport, hints: hints)

        watcher.track(started)

        let selling = await waitUntil { watcher.job(for: self.started.cabalID)?.status == .selling }
        XCTAssertTrue(selling)
        XCTAssertEqual(watcher.job(for: started.cabalID)?.progress, "Selling your slice…")
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user("me"), what: "cashout_changed", id: "1"))

        let done = await waitUntil { watcher.notice != nil }
        XCTAssertTrue(done)
        XCTAssertEqual(watcher.notice?.message, "Cashed out $1.00. It's in your balance.")
        XCTAssertNil(watcher.job(for: started.cabalID))
        let unsubscribed = await waitUntil { await hints.subscriberCount == 0 && !watcher.isObserving }
        XCTAssertTrue(unsubscribed)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [jobPath, jobPath])
    }

    func testOtherUserHintsDoNotReadTheJob() async throws {
        let transport = StubTransport(scripted: [
            try CashOutModelTests.json(.ok, Components.Schemas.CashOutJob.sample(status: .started))
        ])
        let hints = FakeHintStream()
        let watcher = makeWatcher(transport, hints: hints)
        watcher.track(started)
        _ = await waitUntil { await transport.sent.count == 1 }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.changed(.user("me"), what: "balance_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
        XCTAssertNotNil(watcher.job(for: started.cabalID))
    }

    func testAResyncReadsTheJob() async throws {
        let transport = StubTransport(scripted: [
            try CashOutModelTests.json(.ok, Components.Schemas.CashOutJob.sample(status: .started)),
            try CashOutModelTests.json(
                .ok, Components.Schemas.CashOutJob.sample(status: .partial, payoutMicros: "600000")),
        ])
        let hints = FakeHintStream()
        let watcher = makeWatcher(transport, hints: hints)
        watcher.track(started)
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.resync)

        let done = await waitUntil { watcher.notice != nil }
        XCTAssertTrue(done)
        XCTAssertEqual(
            watcher.notice?.message, "Cashed out $0.60, what the sale raised. You keep the shares it didn't cover.")
    }

    func testAJobThatAlreadyEndedNeverSubscribes() async throws {
        let transport = StubTransport(scripted: [])
        let hints = FakeHintStream()
        let watcher = makeWatcher(transport, hints: hints)
        let failed = CashOutJob(
            id: started.id, cabalID: started.cabalID, status: .failed, payoutMicros: 0, resultCode: nil)

        watcher.track(failed)

        XCTAssertEqual(watcher.notice?.message, "Your cash out didn't go through.")
        XCTAssertEqual(watcher.notice?.isSuccess, false)
        XCTAssertFalse(watcher.isObserving)
        let count = await hints.subscriberCount
        XCTAssertEqual(count, 0)
    }

    func testResetDropsTheJobAndTheSubscription() async throws {
        let transport = StubTransport(.hang)
        let hints = FakeHintStream()
        let watcher = makeWatcher(transport, hints: hints)
        watcher.track(started)

        watcher.reset()

        XCTAssertNil(watcher.job(for: started.cabalID))
        XCTAssertFalse(watcher.isObserving)
    }

    private func makeWatcher(_ transport: StubTransport, hints: FakeHintStream) -> CashOutJobWatcher {
        CashOutJobWatcher(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
