import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class InviteLinkModelTests: XCTestCase {
    private static let locked =
        #"{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","handle_link":null,"handle_unlocked":false}"#
    private static let unlocked =
        #"{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","#
        + #""handle_link":"https://monacolabs.xyz/r/kaicenat","handle_unlocked":true}"#
    private static let unlockedWithoutHandleLink =
        #"{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","handle_link":null,"handle_unlocked":true}"#
    private static let codeURL = URL(string: "https://monacolabs.xyz/r/k7m4qx2p")
    private static let handleURL = URL(string: "https://monacolabs.xyz/r/kaicenat")

    func testLockedSharesTheRandomLinkAndPromptsTheUnlock() async throws {
        let (model, transport, _, _) = try make([.json(.ok, Self.locked)])

        await model.load()

        let links = try XCTUnwrap(model.links)
        XCTAssertEqual(links.shareURL, Self.codeURL)
        XCTAssertNil(links.codeURL)
        XCTAssertTrue(links.showsUnlockPrompt)
        XCTAssertEqual(InviteLinks.withoutScheme(links.shareURL), "monacolabs.xyz/r/k7m4qx2p")
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/referral-code"])
    }

    func testUnlockedSharesTheHandleLinkAndKeepsTheRandomLinkBelow() async throws {
        let (model, _, _, _) = try make([.json(.ok, Self.unlocked)])

        await model.load()

        let links = try XCTUnwrap(model.links)
        XCTAssertEqual(links.shareURL, Self.handleURL)
        XCTAssertEqual(links.codeURL, Self.codeURL)
        XCTAssertFalse(links.showsUnlockPrompt)
    }

    func testUnlockedWithANullHandleLinkFallsBackToTheRandomLink() async throws {
        let (model, _, _, _) = try make([.json(.ok, Self.unlockedWithoutHandleLink)])

        await model.load()

        let links = try XCTUnwrap(model.links)
        XCTAssertEqual(links.shareURL, Self.codeURL)
        XCTAssertNil(links.codeURL)
        XCTAssertFalse(links.showsUnlockPrompt)
    }

    func testPendingWaitsTwoSecondsThenLoads() async throws {
        let (model, transport, _, sleeps) = try make([try Self.pending(), .json(.ok, Self.locked)])

        await model.load()

        let durations = await sleeps.durations
        XCTAssertEqual(durations, [.seconds(2)])
        XCTAssertEqual(model.links?.shareURL, Self.codeURL)
        XCTAssertNil(model.toast)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testPendingShowsThePendingStateDuringTheWait() async throws {
        let transport = StubTransport(scripted: [try Self.pending(), .json(.ok, Self.locked)])
        let gate = SleepGate()
        let model = InviteLinkModel(api: api(transport), hints: FakeHintStream(), sleep: { _ in await gate.wait() })

        let load = Task { await model.load() }
        let waiting = await waitUntil { await gate.isWaiting }
        XCTAssertTrue(waiting)
        XCTAssertEqual(model.state, .pending)

        await gate.open()
        await load.value
        XCTAssertNotNil(model.links)
    }

    func testPendingTwiceFailsWithTheServerMessageAndRetriesOnlyOnce() async throws {
        let (model, transport, _, sleeps) = try make([try Self.pending(), try Self.pending()])

        await model.load()

        guard case .failed(.problem(let problem)) = model.state else {
            return XCTFail("want a problem failure, got \(model.state)")
        }
        XCTAssertEqual(problem.code, .known(.referralCodePending))
        XCTAssertEqual(model.toast, "Your invite code is on its way.")
        let sleepCount = await sleeps.durations.count
        XCTAssertEqual(sleepCount, 1)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAFailedFirstLoadToastsAndTheNextLoadRecovers() async throws {
        let (model, _, _, _) = try make([.failure(URLError(.notConnectedToInternet)), .json(.ok, Self.locked)])

        await model.load()

        guard case .failed(.transport) = model.state else {
            return XCTFail("want a transport failure, got \(model.state)")
        }
        XCTAssertEqual(model.toast, "You're offline. Try again.")

        await model.load()

        XCTAssertNotNil(model.links)
    }

    func testAFailedReloadKeepsTheLinks() async throws {
        let (model, _, _, _) = try make([.json(.ok, Self.locked), .failure(URLError(.networkConnectionLost))])
        await model.load()

        await model.load()

        XCTAssertEqual(model.links?.shareURL, Self.codeURL)
        XCTAssertEqual(model.toast, "You're offline. Try again.")
    }

    func testAMeChangedHintReloadsSoADepositFlipsTheScreen() async throws {
        let (model, transport, hints, _) = try make([.json(.ok, Self.locked), .json(.ok, Self.unlocked)])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user("me"), what: "balance_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let afterOtherHint = await transport.sent.count
        XCTAssertEqual(afterOtherHint, 1)

        await hints.send(.changed(.user("me"), what: "me_changed", id: "2"))

        let flipped = await waitUntil { model.links?.shareURL == Self.handleURL }
        XCTAssertTrue(flipped)
    }

    func testAHiddenScreenReloadsOnlyOnceVisibleAgain() async throws {
        let (model, transport, hints, _) = try make([.json(.ok, Self.locked), .json(.ok, Self.unlocked)])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        model.setVisible(false)
        await hints.send(.changed(.user("me"), what: "me_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)

        model.setVisible(true)
        let flipped = await waitUntil { model.links?.shareURL == Self.handleURL }
        XCTAssertTrue(flipped)
    }

    func testUnlockCopyNamesTheHandle() {
        XCTAssertEqual(
            InviteLinks.unlockCopy(handle: "kaicenat"), "Make your first deposit to use @kaicenat as your invite link")
    }

    func testUnlockCopyWithoutAHandle() {
        XCTAssertEqual(
            InviteLinks.unlockCopy(handle: nil), "Make your first deposit to use your handle as your invite link")
    }

    private static func pending() throws -> StubTransport.Reply {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 503, code: .referralCodePending,
                message: "Your invite code is on its way.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: true))
    }

    private func make(
        _ replies: [StubTransport.Reply]
    ) throws -> (InviteLinkModel, StubTransport, FakeHintStream, SleepLog) {
        let transport = StubTransport(scripted: replies)
        let hints = FakeHintStream()
        let sleeps = SleepLog()
        let model = InviteLinkModel(api: api(transport), hints: hints, sleep: { await sleeps.record($0) })
        return (model, transport, hints, sleeps)
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
}

private actor SleepLog {
    private(set) var durations: [Duration] = []

    func record(_ duration: Duration) {
        durations.append(duration)
    }
}

private actor SleepGate {
    private var continuation: CheckedContinuation<Void, Never>?

    var isWaiting: Bool { continuation != nil }

    func wait() async {
        await withCheckedContinuation { continuation = $0 }
    }

    func open() {
        continuation?.resume()
        continuation = nil
    }
}
