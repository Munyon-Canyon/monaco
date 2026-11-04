import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ChatProposalStoreTests: XCTestCase {
    private nonisolated static func facts(yes: Int = 0, status: Components.Schemas.ProposalStatus = .open)
        -> ChatProposalFacts
    {
        var proposal = Components.Schemas.Proposal.sample(status: status)
        proposal.id = "p1"
        proposal.tally = .init(yes: yes, no: 0, voters: 3, needed: 2)
        return ChatProposalFacts(summary: ProposalSummary(proposal), asset: nil)
    }

    private func loaded(_ store: ChatProposalStore, _ id: String = "p1") -> ChatProposalFacts? {
        if case .loaded(let facts) = store.state(for: id) { return facts }
        return nil
    }

    func testAnIdIsFetchedOncePerSession() async {
        let counter = Counter()
        let facts = Self.facts()
        let store = ChatProposalStore { _ in
            await counter.bump()
            return facts
        }

        XCTAssertEqual(store.state(for: "p1"), .loading)
        await store.load("p1")
        await store.load("p1")

        let fetches = await counter.value
        XCTAssertEqual(fetches, 1)
        XCTAssertEqual(loaded(store), facts)
    }

    func testAFailedFetchShowsFailedAndRetryLoadsIt() async {
        let counter = Counter()
        let facts = Self.facts()
        let store = ChatProposalStore { _ in
            let attempt = await counter.bump()
            if attempt == 1 { throw URLError(.notConnectedToInternet) }
            return facts
        }

        await store.load("p1")
        XCTAssertEqual(store.state(for: "p1"), .failed)
        await store.retry("p1")

        XCTAssertEqual(loaded(store), facts)
    }

    func testRetryDoesNothingUnlessTheLoadFailed() async {
        let counter = Counter()
        let facts = Self.facts()
        let store = ChatProposalStore { _ in
            await counter.bump()
            return facts
        }

        await store.retry("p1")
        let fetches = await counter.value
        XCTAssertEqual(fetches, 0)
        XCTAssertEqual(store.state(for: "p1"), .loading)
    }

    func testReloadUpdatesTheTallyOfALoadedId() async {
        let counter = Counter()
        let store = ChatProposalStore { _ in
            let attempt = await counter.bump()
            return Self.facts(yes: attempt)
        }

        await store.load("p1")
        await store.reload(["p1", "never-loaded"])

        XCTAssertEqual(loaded(store)?.summary.tally.yes, 2)
        XCTAssertEqual(store.state(for: "never-loaded"), .loading)
        let fetches = await counter.value
        XCTAssertEqual(fetches, 2)
    }

    func testReloadKeepsTheOldStateWhenTheFetchFails() async {
        let counter = Counter()
        let first = Self.facts(yes: 1)
        let store = ChatProposalStore { _ in
            let attempt = await counter.bump()
            if attempt > 1 { throw URLError(.timedOut) }
            return first
        }

        await store.load("p1")
        await store.reload(["p1"])

        XCTAssertEqual(loaded(store), first)
    }

    func testAProposalHintReloadsTheLoadedProposal() async {
        let counter = Counter()
        let store = ChatProposalStore { _ in Self.facts(yes: await counter.bump()) }
        let hints = ScriptedHints([
            .changed(.cabal("c1"), what: "proposal_updated", id: "p1"),
            .changed(.cabal("c1"), what: "proposal_updated", id: "other"),
        ])

        await store.load("p1")
        await store.observe(cabalID: "c1", hints: hints)

        XCTAssertEqual(loaded(store)?.summary.tally.yes, 2)
        XCTAssertEqual(store.state(for: "other"), .loading)
    }

    func testTheCardCopyIsExact() {
        XCTAssertEqual(GroupChatCopy.sent, "Sent")
        XCTAssertEqual(GroupChatCopy.sending, "Sending…")
        XCTAssertEqual(GroupChatCopy.notSent, "Not sent")
        XCTAssertEqual(GroupChatCopy.retry, "Retry")
        XCTAssertEqual(GroupChatCopy.offline, "You're offline. Messages can be drafted but won't send yet.")
        XCTAssertEqual(GroupChatCopy.statusLine(time: "10:25", status: GroupChatCopy.sent), "10:25 · Sent")
    }
}

private struct ScriptedHints: HintSource {
    let hints: [Hint]
    init(_ hints: [Hint]) { self.hints = hints }

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            for hint in hints { continuation.yield(hint) }
            continuation.finish()
        }
    }
}

private actor Counter {
    private(set) var value = 0

    @discardableResult
    func bump() -> Int {
        value += 1
        return value
    }
}
