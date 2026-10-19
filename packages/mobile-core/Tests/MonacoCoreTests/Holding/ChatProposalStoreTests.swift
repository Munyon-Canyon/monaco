import Foundation
import MonacoAPI
import MonacoCore
import XCTest

@MainActor
final class ChatProposalStoreTests: XCTestCase {
    private func facts(phase: ChatProposalFacts.Phase = .open, isSell: Bool = false) -> ChatProposalFacts {
        ChatProposalFacts(
            proposalID: "p1", proposerID: "u1", assetName: "NVIDIA", isSell: isSell, amountMicros: 1_200_000_000,
            createdAt: Date(timeIntervalSince1970: 1_790_245_080), phase: phase)
    }

    func testACardReadsTheProposerTheAssetAndTheAmount() {
        let open = facts()

        XCTAssertEqual(open.title(proposer: "Mia"), "Mia proposed NVIDIA")
        XCTAssertEqual(open.amountLine, "$1,200.00 from the cabal pot")
        XCTAssertEqual(open.buttonTitle, "Review")
        XCTAssertEqual(facts(isSell: true).title(proposer: "Mia"), "Mia proposed selling NVIDIA")
    }

    func testAnApprovedProposalSaysSoAndOffersStatus() {
        let approved = facts(phase: .approved)

        XCTAssertEqual(approved.title(proposer: "Mia"), "Proposal approved")
        XCTAssertEqual(approved.buttonTitle, "Status")
        XCTAssertEqual(facts(phase: .closed).buttonTitle, "Status")
    }

    func testAnIdIsFetchedOncePerSession() async {
        let counter = Counter()
        let store = ChatProposalStore { _ in
            await counter.bump()
            return ChatProposalFacts(
                proposalID: "p1", proposerID: "u1", assetName: "NVIDIA", isSell: false, amountMicros: nil,
                createdAt: Date(timeIntervalSince1970: 0), phase: .open)
        }

        XCTAssertEqual(store.state(for: "p1"), .loading)
        await store.load("p1")
        await store.load("p1")

        let fetches = await counter.value
        XCTAssertEqual(fetches, 1)
        guard case .loaded = store.state(for: "p1") else { return XCTFail("got \(store.state(for: "p1"))") }
    }

    func testAFailedFetchLeavesTheCardWithTheFallbackAndCanBeRetried() async {
        let counter = Counter()
        let store = ChatProposalStore { _ in
            let attempt = await counter.bump()
            if attempt == 1 { throw URLError(.notConnectedToInternet) }
            return ChatProposalFacts(
                proposalID: "p1", proposerID: "u1", assetName: "NVIDIA", isSell: false, amountMicros: nil,
                createdAt: Date(timeIntervalSince1970: 0), phase: .open)
        }

        await store.load("p1")
        XCTAssertEqual(store.state(for: "p1"), .failed)
        await store.load("p1")

        guard case .loaded = store.state(for: "p1") else { return XCTFail("got \(store.state(for: "p1"))") }
    }

    func testTheCardCopyIsExact() {
        XCTAssertEqual(GroupChatCopy.unknownProposal, "A proposal")
        XCTAssertEqual(GroupChatCopy.executionPending, "Execution pending · Not completed")
        XCTAssertEqual(GroupChatCopy.sent, "Sent")
        XCTAssertEqual(GroupChatCopy.sending, "Sending…")
        XCTAssertEqual(GroupChatCopy.notSent, "Not sent")
        XCTAssertEqual(GroupChatCopy.retry, "Retry")
        XCTAssertEqual(GroupChatCopy.offline, "You're offline. Messages can be drafted but won't send yet.")
        XCTAssertEqual(GroupChatCopy.statusLine(time: "10:25", status: GroupChatCopy.sent), "10:25 · Sent")
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
