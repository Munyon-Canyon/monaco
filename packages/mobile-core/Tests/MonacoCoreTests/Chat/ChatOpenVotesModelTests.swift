import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ChatOpenVotesModelTests: XCTestCase {
    private nonisolated static let now = Date(timeIntervalSince1970: 1_800_000_000)
    private var now: Date { Self.now }

    private nonisolated static func vote(
        _ id: String, closesIn: TimeInterval, status: Components.Schemas.ProposalStatus = .open,
        yes: Int = 0, no: Int = 0, voters: Int = 8, replies: Int = 0, age: TimeInterval = 100
    ) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample(status: status)
        proposal.id = id
        proposal.expiresAt = Self.now.addingTimeInterval(closesIn)
        proposal.createdAt = Self.now.addingTimeInterval(-age)
        proposal.tally = .init(yes: yes, no: no, voters: voters, needed: 5)
        proposal.replyCount = replies
        return ProposalSummary(proposal)
    }

    private func model(_ pages: [String?: ChatOpenVotesModel.Page]) -> ChatOpenVotesModel {
        ChatOpenVotesModel(cabalID: "c1") { cursor in
            guard let page = pages[cursor] else { throw URLError(.badServerResponse) }
            return page
        }
    }

    func testLoadKeepsOnlyOpenVotesSoonestClosingFirstAcrossPages() async {
        let sut = model([
            nil: ([Self.vote("late", closesIn: 5 * 3600), Self.vote("done", closesIn: 100, status: .passed)], "next"),
            "next": ([Self.vote("soon", closesIn: 4 * 3600)], nil),
        ])

        await sut.load()

        XCTAssertEqual(sut.votes.map(\.id), ["soon", "late"])
        XCTAssertEqual(sut.count, 2)
    }

    func testVotesClosingTogetherAreOrderedByCreation() async {
        let sut = model([
            nil: ([Self.vote("newer", closesIn: 3600, age: 10), Self.vote("older", closesIn: 3600, age: 500)], nil)
        ])

        await sut.load()

        XCTAssertEqual(sut.votes.map(\.id), ["older", "newer"])
    }

    func testLoadStopsAfterFivePages() async {
        let counter = PageCounter()
        let sut = ChatOpenVotesModel(cabalID: "c1") { _ in
            let n = await counter.bump()
            return ([Self.vote("v\(n)", closesIn: TimeInterval(n * 60))], "more")
        }

        await sut.load()

        let pages = await counter.value
        XCTAssertEqual(pages, 5)
        XCTAssertEqual(sut.count, 5)
    }

    func testAFailedLoadKeepsTheVotesAlreadyShown() async {
        let counter = PageCounter()
        let sut = ChatOpenVotesModel(cabalID: "c1") { _ in
            let n = await counter.bump()
            if n > 1 { throw URLError(.timedOut) }
            return ([Self.vote("a", closesIn: 3600)], nil)
        }

        await sut.load()
        await sut.reload()

        XCTAssertEqual(sut.votes.map(\.id), ["a"])
    }

    func testAClosedVoteDropsOutOnAHintAndTheNextBecomesTheLead() async {
        let counter = PageCounter()
        let sut = ChatOpenVotesModel(cabalID: "c1") { _ in
            let n = await counter.bump()
            let nvda = Self.vote("nvda", closesIn: 4 * 3600, status: n == 1 ? .open : .passed)
            return ([nvda, Self.vote("aapl", closesIn: 5 * 3600)], nil)
        }

        await sut.load()
        XCTAssertEqual(sut.lead(hiding: [])?.id, "nvda")

        await sut.observe(
            hints: StubHints([.changed(.cabal("c1"), what: "proposal_updated", id: "nvda"), .resync]))

        XCTAssertEqual(sut.votes.map(\.id), ["aapl"])
        XCTAssertEqual(sut.lead(hiding: [])?.id, "aapl")
    }

    func testLeadSkipsVotesWhoseCardsAreVisible() async {
        let sut = model([nil: ([Self.vote("a", closesIn: 3600), Self.vote("b", closesIn: 7200)], nil)])
        await sut.load()

        XCTAssertEqual(sut.lead(hiding: ["a"])?.id, "b")
        XCTAssertNil(sut.lead(hiding: ["a", "b"]))
    }

    func testLeadIsNilWhenNothingIsOpen() async {
        let sut = model([nil: ([], nil)])
        await sut.load()

        XCTAssertNil(sut.lead(hiding: []))
        XCTAssertEqual(sut.count, 0)
    }

    func testCapsuleLine() {
        let nvda = Self.vote("a", closesIn: 4 * 3600, yes: 4, no: 1, voters: 8, replies: 6)
        let aapl = Self.vote("b", closesIn: 5 * 3600, yes: 1, no: 1, voters: 8, replies: 1)
        let quiet = Self.vote("c", closesIn: 6 * 3600, voters: 3)
        let minutes = Self.vote("d", closesIn: 35 * 60, voters: 3)
        let last = Self.vote("e", closesIn: 20, voters: 3)

        XCTAssertEqual(ChatOpenVotesModel.capsuleLine(for: nvda, now: now), "5 of 8 voted · 6 replies · 4h")
        XCTAssertEqual(ChatOpenVotesModel.capsuleLine(for: aapl, now: now), "2 of 8 voted · 1 reply · 5h")
        XCTAssertEqual(ChatOpenVotesModel.capsuleLine(for: quiet, now: now), "0 of 3 voted · 6h")
        XCTAssertEqual(ChatOpenVotesModel.capsuleLine(for: minutes, now: now), "0 of 3 voted · 35m")
        XCTAssertEqual(ChatOpenVotesModel.capsuleLine(for: last, now: now), "0 of 3 voted · <1m")
    }

    func testSpokenLine() {
        let nvda = Self.vote("a", closesIn: 4 * 3600, yes: 4, no: 1, voters: 8, replies: 6)
        let aapl = Self.vote("b", closesIn: 3600, yes: 1, no: 1, voters: 8, replies: 1)
        let quiet = Self.vote("c", closesIn: 20, voters: 3)

        XCTAssertEqual(
            ChatOpenVotesModel.spokenLine(for: nvda, now: now), "5 of 8 voted, 6 replies, closes in 4 hours")
        XCTAssertEqual(
            ChatOpenVotesModel.spokenLine(for: aapl, now: now), "2 of 8 voted, 1 reply, closes in 1 hour")
        XCTAssertEqual(
            ChatOpenVotesModel.spokenLine(for: quiet, now: now), "0 of 3 voted, closes in less than a minute")
    }

    func testSectionTitle() {
        XCTAssertEqual(ChatOpenVotesModel.sectionTitle, "Open votes")
    }
}

private struct StubHints: HintSource {
    let hints: [Hint]
    init(_ hints: [Hint]) { self.hints = hints }

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            for hint in hints { continuation.yield(hint) }
            continuation.finish()
        }
    }
}

private actor PageCounter {
    private(set) var value = 0

    @discardableResult
    func bump() -> Int {
        value += 1
        return value
    }
}
