import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class FeedVoteCardTests: XCTestCase {
    private let clock = Date(timeIntervalSince1970: 1_999_000_000)

    private func proposal(
        action: String = "buy", hours: Double = 4, ballot: Components.Schemas.FeedItem.MyBallotPayload? = nil,
        canVote: Bool = true, status: String? = nil
    ) -> Components.Schemas.FeedItem {
        var item = FeedPreviewFixtures.items[0]
        item.action = action
        item.status = status
        item.myBallot = ballot
        item.canVote = canVote
        item.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        item.expiresAt = clock.addingTimeInterval(hours * 3600)
        item.symbol = "NVDAx"
        item.cabalName = "AC Capital"
        item.usdcMicros = 400_000_000
        item.tokenAmount = 250_000_000
        item.tokenDecimals = 8
        return item
    }

    private func card(_ item: Components.Schemas.FeedItem) throws -> HomeVoteCardModel {
        let row = try XCTUnwrap(FeedVoteCard.row(for: item))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: StubTransport(scripted: []))
        return HomeVoteCardModel(
            api: api, hints: FakeHintStream(), row: row, now: { [clock] in clock },
            myBallot: FeedVoteCard.ballot(of: item), showsReplies: false)
    }

    private func page(_ items: [Components.Schemas.FeedItem]) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = try encoder.encode(Components.Schemas.FeedPage(items: items, nextCursor: nil))
        return .json(.ok, String(decoding: body, as: UTF8.self))
    }

    func testFeedModelKeepsOneCardPerProposalAndUpdatesItsTally() async throws {
        let first = proposal()
        var later = first
        later.tally = .init(yes: 5, no: 1, voters: 8, needed: 5)
        let transport = StubTransport(scripted: [try page([first]), try page([later])])
        let model = FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport),
            viewerID: nil, hints: FakeHintStream(), clock: TestClock())

        await model.load()
        let card = try XCTUnwrap(model.voteCard(for: first))
        XCTAssertEqual(card.tally.yes, 4)
        XCTAssertNil(model.voteCard(for: proposal(canVote: false)))

        await model.load()
        XCTAssertTrue(try XCTUnwrap(model.voteCard(for: later)) === card)
        XCTAssertEqual(card.tally.yes, 5)
    }

    func testBuyTitleAndLineWithHours() async throws {
        let model = try card(proposal())
        XCTAssertEqual(model.headline, "Buy NVDA \u{00B7} $400")
        XCTAssertEqual(
            model.subheadLine(now: clock, showsCabal: true).short, "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 4h")
    }

    func testSellTitle() async throws {
        let model = try card(proposal(action: "sell"))
        XCTAssertTrue(model.headline.hasPrefix("Sell NVDA \u{00B7} "))
    }

    func testLineWithDaysAndClosed() async throws {
        let days = try card(proposal(hours: 52))
        XCTAssertTrue(days.subheadLine(now: clock, showsCabal: true).short.hasSuffix("2d"))
        let closed = try card(proposal(hours: -1))
        XCTAssertTrue(closed.subheadLine(now: clock, showsCabal: true).short.hasSuffix("voting closed"))
    }

    func testLineNeverCarriesReplyCount() async throws {
        let row = try XCTUnwrap(FeedVoteCard.row(for: proposal()))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: StubTransport(scripted: []))
        let model = HomeVoteCardModel(
            api: api, hints: FakeHintStream(),
            row: HomePendingVote(
                id: row.id, cabalID: row.cabalID, cabalName: row.cabalName, kind: row.kind, symbol: row.symbol,
                tally: row.tally, replyCount: 6, expiresAt: row.expiresAt),
            now: { [clock] in clock }, showsReplies: false)
        XCTAssertFalse(model.subheadLine(now: clock, showsCabal: true).short.contains("repl"))
    }

    func testSeedFromMyBallot() async throws {
        let voted = try card(proposal(ballot: .no))
        XCTAssertTrue(voted.isCommitted)
        XCTAssertEqual(voted.selectedSide, .no)
        XCTAssertEqual(voted.tally.yes, 4)
        XCTAssertFalse(voted.canUndo)
        let fresh = try card(proposal())
        XCTAssertEqual(fresh.phase, .idle)
    }

    func testRowRejectsOtherKindsAndMissingFields() async throws {
        var item = proposal()
        item.kind = "result"
        XCTAssertNil(FeedVoteCard.row(for: item))
        item = proposal(action: "rule")
        XCTAssertNil(FeedVoteCard.row(for: item))
        item = proposal()
        item.tally = nil
        XCTAssertNil(FeedVoteCard.row(for: item))
        item = proposal()
        item.expiresAt = nil
        XCTAssertNil(FeedVoteCard.row(for: item))
        item = proposal()
        item.proposalId = nil
        XCTAssertNil(FeedVoteCard.row(for: item))
        item = proposal()
        item.cabalId = nil
        XCTAssertNil(FeedVoteCard.row(for: item))
    }

    func testVotableNeedsCanVoteOrBallot() async throws {
        XCTAssertTrue(FeedVoteCard.isVotable(proposal()))
        XCTAssertTrue(FeedVoteCard.isVotable(proposal(ballot: .yes, canVote: false)))
        XCTAssertFalse(FeedVoteCard.isVotable(proposal(canVote: false)))
    }

    func testReadOnlyLine() async throws {
        XCTAssertEqual(
            FeedVoteCard.readOnlyLine(for: proposal(canVote: false), now: clock),
            "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 4h")
        XCTAssertNil(FeedVoteCard.readOnlyLine(for: proposal(canVote: false, status: "passed"), now: clock))
        var item = proposal()
        item.tally = nil
        XCTAssertNil(FeedVoteCard.readOnlyLine(for: item, now: clock))
    }
}
