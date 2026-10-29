import Foundation
import HTTPTypes
import MonacoAPI
import MonacoNotify
import MonacoTestSupport
import XCTest

@MainActor
final class CatchUpDeckTests: XCTestCase {
    private nonisolated static let now = Components.Schemas.CatchUp.sampleNow
    private static let nvda = "proposal:00000000-0000-7000-8000-00000000b001"
    private static let christelle = "comment:00000000-0000-7000-8000-00000000d001"
    private static let mia = "comment:00000000-0000-7000-8000-00000000d002"
    private static let sam = "invite:00000000-0000-7000-8000-00000000e001"
    private static let meta = "proposal:00000000-0000-7000-8000-00000000b004"
    private static let tsla = "proposal:00000000-0000-7000-8000-00000000b005"

    private func screen(_ read: Components.Schemas.CatchUp = .deck) -> CatchUpScreen {
        CatchUpScreen(read)
    }

    private func keys(_ items: [Components.Schemas.CatchUpItem]) -> [String] {
        items.map(\.key)
    }

    func testTheOrderIsWaitingRepliesInvitesResults() async {
        let deck = CatchUpDeck(screen: screen())

        let order: [String] = keys(deck.items)

        XCTAssertEqual(order, [Self.nvda, Self.christelle, Self.mia, Self.sam, Self.meta, Self.tsla])
        XCTAssertEqual(deck.total, 6)
    }

    func testItOpensOnTheFirstUnreadItemOrTheFirstItem() async {
        let unread = CatchUpDeck(screen: screen())
        XCTAssertEqual(unread.current?.key, Self.nvda)

        let none: [Components.Schemas.CatchUpItem] = screen().activeItems.map { item in
            var copy = item
            copy.unread = false
            return copy
        }
        let allRead = CatchUpDeck(items: none)
        XCTAssertEqual(allRead.current?.key, Self.nvda)

        var someRead = none
        someRead[2].unread = true
        XCTAssertEqual(CatchUpDeck(items: someRead).current?.key, Self.mia)
    }

    func testStartingAtAKeyOpensThereAndCountsFromTheWholeDeck() async {
        let deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)

        XCTAssertEqual(deck.current?.key, Self.christelle)
        XCTAssertEqual(deck.position, "2 of 6")
    }

    func testUpNextIsTheFollowingFourAndShrinksAtTheEnd() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)

        XCTAssertEqual(keys(deck.upNext), [Self.mia, Self.sam, Self.meta, Self.tsla])

        deck.advance()
        XCTAssertEqual(keys(deck.upNext), [Self.sam, Self.meta, Self.tsla])
        XCTAssertEqual(deck.position, "3 of 6")

        let first = CatchUpDeck(screen: screen(), startingAt: Self.nvda)
        XCTAssertEqual(first.upNext.count, 4)
    }

    func testAdvancingPastTheEndFinishesAndStaysFinished() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.tsla)

        XCTAssertEqual(deck.position, "6 of 6")
        XCTAssertFalse(deck.isFinished)
        deck.advance()
        XCTAssertTrue(deck.isFinished)
        XCTAssertNil(deck.current)
        XCTAssertTrue(deck.upNext.isEmpty)
        deck.advance()
        XCTAssertTrue(deck.isFinished)
        XCTAssertEqual(deck.position, "6 of 6")
    }

    func testAdvancingPastAKeyOnlyMovesWhenItIsTheCurrentItem() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)

        deck.advance(past: Self.mia)
        XCTAssertEqual(deck.current?.key, Self.christelle)
        deck.advance(past: Self.christelle)
        XCTAssertEqual(deck.current?.key, Self.mia)
    }

    func testAnItemClearedElsewhereAdvancesWithoutSkippingTheNextOne() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)
        let elsewhere = Self.removing(Self.christelle)

        deck.reconcile(with: elsewhere)

        XCTAssertEqual(deck.current?.key, Self.mia)
        XCTAssertEqual(deck.position, "3 of 6")
        XCTAssertEqual(keys(deck.upNext), [Self.sam, Self.meta, Self.tsla])
    }

    func testAnItemClearedElsewhereAheadIsLeftOutOfUpNextAndSkippedOver() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)
        let elsewhere = Self.removing(Self.mia)

        deck.reconcile(with: elsewhere)

        XCTAssertEqual(deck.current?.key, Self.christelle)
        XCTAssertEqual(keys(deck.upNext), [Self.sam, Self.meta, Self.tsla])
        deck.advance()
        XCTAssertEqual(deck.current?.key, Self.sam)
    }

    func testReconcilingRefreshesTheItemsAndKeepsPlaceWhenNothingChanged() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.christelle)

        deck.reconcile(with: screen())

        XCTAssertEqual(deck.current?.key, Self.christelle)
        XCTAssertEqual(deck.position, "2 of 6")
    }

    func testClearingTheLastItemElsewhereFinishesTheDeck() async {
        var deck = CatchUpDeck(screen: screen(), startingAt: Self.tsla)
        let elsewhere = Self.removing(Self.tsla)

        deck.reconcile(with: elsewhere)

        XCTAssertTrue(deck.isFinished)
    }

    func testAnEmptyScreenHasNoCurrentItem() async {
        let deck = CatchUpDeck(screen: CatchUpScreen(.caughtUp))

        XCTAssertNil(deck.current)
        XCTAssertTrue(deck.isFinished)
        XCTAssertEqual(deck.total, 0)
    }

    private nonisolated static func removing(_ key: String) -> CatchUpScreen {
        var read = Components.Schemas.CatchUp.deck
        read.waiting.removeAll { $0.key == key }
        read.replies.removeAll { $0.key == key }
        read.invites.removeAll { $0.key == key }
        read.results.removeAll { $0.key == key }
        read.count = Int(read.waiting.count + read.replies.count + read.invites.count + read.results.count)
        return CatchUpScreen(read)
    }
}

@MainActor
final class CatchUpDeckModelTests: XCTestCase {
    private nonisolated static let now = Components.Schemas.CatchUp.sampleNow
    private static let christelle = "comment:00000000-0000-7000-8000-00000000d001"
    private static let mia = "comment:00000000-0000-7000-8000-00000000d002"

    private static func body() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(Components.Schemas.CatchUp.deck), as: UTF8.self)
    }

    private func make(
        _ replies: [StubTransport.Reply]
    ) async throws -> (CatchUpDeckModel, CatchUpModel, StubTransport) {
        let transport = StubTransport(scripted: [.json(.ok, try Self.body())] + replies)
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        let catchUp = CatchUpModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream(), clock: { Self.now }, calendar: calendar)
        await catchUp.load()
        guard case .loaded(let screen) = catchUp.state else { throw XCTSkip("not loaded") }
        return (CatchUpDeckModel(catchUp: catchUp, screen: screen, startingAt: Self.christelle), catchUp, transport)
    }

    private func noContent() -> StubTransport.Reply {
        .response(status: .noContent, contentType: "application/json", body: Data())
    }

    private func posts(_ transport: StubTransport) async -> [String?] {
        await transport.sent.filter { $0.method == .post }.map(\.path)
    }

    private func encoded(_ key: String) -> String {
        key.replacingOccurrences(of: ":", with: "%3A")
    }

    func testClearSendsOneClearForTheKeyAndShowsTheNextCard() async throws {
        let (model, catchUp, transport) = try await make([noContent()])

        let ok = await model.clear()
        model.sync()

        XCTAssertTrue(ok)
        let sent: [String?] = await posts(transport)
        XCTAssertEqual(sent, ["/v1/me/catch-up/items/\(encoded(Self.christelle))/clear"])
        XCTAssertEqual(model.deck.current?.key, Self.mia)
        XCTAssertEqual(model.deck.position, "3 of 6")
        guard case .loaded(let screen) = catchUp.state else { return XCTFail("not loaded") }
        XCTAssertFalse(screen.replies.contains { $0.key == Self.christelle })
    }

    func testKeepSendsOneKeepAdvancesAndLeavesTheItemInTheList() async throws {
        let (model, catchUp, transport) = try await make([noContent()])

        let ok = await model.keep()
        model.sync()

        XCTAssertTrue(ok)
        let sent: [String?] = await posts(transport)
        XCTAssertEqual(sent, ["/v1/me/catch-up/items/\(encoded(Self.christelle))/keep"])
        XCTAssertEqual(model.deck.current?.key, Self.mia)
        guard case .loaded(let screen) = catchUp.state else { return XCTFail("not loaded") }
        let kept: Components.Schemas.CatchUpItem? = screen.replies.first { $0.key == Self.christelle }
        XCTAssertEqual(kept?.unread, false)
    }

    func testAFailedClearStillMovesOnAndReportsTheFailure() async throws {
        let (model, _, _) = try await make([.failure(URLError(.notConnectedToInternet))])

        let ok = await model.clear()
        model.sync()

        XCTAssertFalse(ok)
        XCTAssertEqual(model.deck.current?.key, Self.mia)
    }

    func testWorkingThroughTheDeckFinishesIt() async throws {
        let (model, _, _) = try await make(Array(repeating: noContent(), count: 5))

        for _ in 0..<5 {
            await model.clear()
            model.sync()
        }

        XCTAssertTrue(model.deck.isFinished)
        let again = await model.clear()
        XCTAssertFalse(again)
    }
}

final class CatchUpDeckCopyTests: XCTestCase {
    private static let now = Components.Schemas.CatchUp.sampleNow

    private func calendar() -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar
    }

    func testTheProposalLines() {
        let mention: Components.Schemas.CatchUpItem = CatchUpScreen(.deck).replies[0]
        let proposal = mention.proposal
        XCTAssertEqual(
            proposal.map { CatchUpDeckCopy.proposedLine($0, now: Self.now, calendar: calendar()) }, "Mia proposed · 2h")
        XCTAssertEqual(
            proposal.map { CatchUpDeckCopy.statusLine($0, now: Self.now) }, "5 of 8 voted · 6 replies · ends in 4h")
    }

    func testAClosedProposalHasNoEndsInAndTheResultLineNamesYourVote() {
        let result: Components.Schemas.CatchUpItem = CatchUpScreen(.deck).results[0]
        XCTAssertEqual(
            result.proposal.map { CatchUpDeckCopy.statusLine($0, now: Self.now) }, "5 of 5 voted · 6 replies")
        XCTAssertEqual(CatchUpDeckCopy.closedLine(result), "you voted Yes · filled")
    }

    func testThePostHeadAndTheOpenLabelByKind() {
        let screen = CatchUpScreen(.deckPostReply)
        let reply: Components.Schemas.CatchUpItem = screen.replies[0]
        XCTAssertEqual(reply.post.map(CatchUpDeckCopy.postHead), "Logan · sold AAPL")
        XCTAssertEqual(CatchUpDeckCopy.openLabel(reply), "Open post")
        XCTAssertEqual(CatchUpDeckCopy.openLabel(CatchUpScreen(.deck).replies[0]), "Open proposal")
        XCTAssertEqual(CatchUpDeckCopy.openLabel(screen.invites[0]), "Open cabal")
    }

    func testAMentionIsASegmentOfItsOwn() {
        let segments: [CatchUpDeckCopy.Segment] = CatchUpDeckCopy.segments("Still out: @Andres, Jordan?")

        XCTAssertEqual(
            segments,
            [
                .init(text: "Still out: ", isMention: false), .init(text: "@Andres", isMention: true),
                .init(text: ", Jordan?", isMention: false),
            ])
        XCTAssertEqual(CatchUpDeckCopy.segments("mail me at a@b.com").count, 1)
        XCTAssertEqual(CatchUpDeckCopy.segments("@").map(\.isMention), [false])
    }
}
