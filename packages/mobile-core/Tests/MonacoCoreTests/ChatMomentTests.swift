import Foundation
import MonacoAPI
import MonacoTestSupport
import Synchronization
import XCTest

@testable import MonacoCore

@MainActor
final class ChatMomentTests: XCTestCase {
    private static let filledAt = Date(timeIntervalSince1970: 1_790_778_600)
    private static let now = Date(timeIntervalSince1970: 1_791_028_800)
    private static let nextYear = Date(timeIntervalSince1970: 1_791_028_800 + 366 * 86_400)
    private static let nilID = "00000000-0000-0000-0000-000000000000"

    private static var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }

    private static let locale = Locale(identifier: "en_US_POSIX")

    private static func system(
        _ id: String, _ moment: Components.Schemas.ChatMoment?, minutes: Double = 0
    ) -> ChatMessage {
        ChatMessage(
            id: id, kind: .system,
            author: .init(id: nilID, handle: nil, displayName: "", photoUrl: nil),
            moment: moment,
            body: nil,
            createdAt: ChatFixtures.epoch.addingTimeInterval(minutes * 60),
            alsoInChannel: false, replyCount: 0, lastRepliers: [], deleted: false, reactions: [], reactionCount: 0)
    }

    private static func member(_ id: String, _ name: String) -> Components.Schemas.ChatMoment {
        .memberJoined(
            .init(kind: .memberJoined, member: .init(id: id, handle: nil, displayName: name, photoUrl: nil)))
    }

    private static func fill(
        _ side: Components.Schemas.TradeFilledMoment.SidePayload = .buy, symbol: String = "NVDAx",
        price: Int64 = 196_750_000
    ) -> Components.Schemas.ChatMoment {
        .tradeFilled(
            .init(
                kind: .tradeFilled, side: side, symbol: symbol, assetId: "asset-1", priceMicros: price,
                usdcMicros: 300_000_000, filledAt: filledAt, swapId: "swap-1", proposalId: nil))
    }

    private func lines(
        _ moment: Components.Schemas.ChatMoment, viewer: String = ChatFixtures.viewerID, price: Int64? = nil,
        now: Date = now
    )
        throws -> ChatMoment.Lines
    {
        let parsed = try XCTUnwrap(ChatMoment(message: Self.system("s", moment), viewerID: viewer))
        return parsed.lines(
            cabalName: "Big Ballers", currentPriceMicros: price, now: now, calendar: Self.calendar,
            locale: Self.locale)
    }

    func testCabalCreatedReadsAsWasCreated() async throws {
        XCTAssertEqual(try lines(.cabalCreated(.init(kind: .cabalCreated))).title, "Big Ballers was created")
    }

    func testTheViewersOwnJoinReadsYouJoined() async throws {
        let moment = Self.member(ChatFixtures.viewerID, "Logan Norman")
        XCTAssertEqual(try lines(moment).title, "You joined Big Ballers")
    }

    func testAnotherMemberReadsByFirstNameOnly() async throws {
        let moment = Self.member(ChatFixtures.otherID, "Christelle Dupont")
        XCTAssertEqual(try lines(moment).title, "Christelle joined Big Ballers")
    }

    func testAMemberWithoutANameReadsAsSomeone() async throws {
        XCTAssertEqual(try lines(Self.member(ChatFixtures.otherID, "")).title, "Someone joined Big Ballers")
    }

    func testABuyShowsTheDateTheCurrentPriceAndTheSignedReturn() async throws {
        let result = try lines(Self.fill(), price: 220_000_000)
        XCTAssertEqual(result.title, "Bought NVDA at $196.75")
        XCTAssertEqual(result.detail, "Filled Sep 30 · now $220.00")
        XCTAssertEqual(result.change, "+11.8%")
        XCTAssertEqual(result.direction, .up)
        XCTAssertEqual(result.subtitle, "Filled Sep 30 · now $220.00, +11.8%")
    }

    func testABuyBelowTheFillPriceIsDown() async throws {
        let result = try lines(Self.fill(), price: 180_000_000)
        XCTAssertEqual(result.direction, .down)
        XCTAssertEqual(result.change, "\u{2212}8.5%")
    }

    func testABuyAtTheFillPriceIsFlat() async throws {
        let result = try lines(Self.fill(), price: 196_750_000)
        XCTAssertEqual(result.direction, .flat)
        XCTAssertEqual(result.change, "0.0%")
    }

    func testAMissingQuoteDropsTheNowPart() async throws {
        let result = try lines(Self.fill(), price: nil)
        XCTAssertEqual(result.subtitle, "Filled Sep 30")
        XCTAssertNil(result.change)
        XCTAssertNil(result.direction)
    }

    func testASellShowsNoPercent() async throws {
        let result = try lines(Self.fill(.sell), price: 220_000_000)
        XCTAssertEqual(result.title, "Sold NVDA at $196.75")
        XCTAssertEqual(result.subtitle, "Filled Sep 30 · now $220.00")
        XCTAssertNil(result.change)
    }

    func testAZeroFillPriceShowsNoPercent() async throws {
        let result = try lines(Self.fill(price: 0), price: 220_000_000)
        XCTAssertEqual(result.subtitle, "Filled Sep 30 · now $220.00")
    }

    func testAFillFromAnEarlierYearCarriesTheYear() async throws {
        let result = try lines(Self.fill(), price: nil, now: Self.nextYear)
        XCTAssertEqual(result.subtitle, "Filled Sep 30, 2026")
    }

    func testOnlySystemRowsWithAMomentAreMoments() {
        XCTAssertNil(ChatMoment(message: ChatFixtures.message("m1"), viewerID: ChatFixtures.viewerID))
        XCTAssertNil(ChatMoment(message: Self.system("s1", nil), viewerID: ChatFixtures.viewerID))
        XCTAssertNil(ChatMoment.cabalCreated.symbol)
    }

    func testTheTimelineInterleavesMomentsByTimeAndNeverGroupsThem() {
        var timeline = ChatTimeline(viewerID: ChatFixtures.viewerID)
        timeline.mergeNewest(
            [
                ChatFixtures.message("m1", minutes: 0),
                Self.system("s1", Self.fill(), minutes: 1),
                ChatFixtures.message("m3", minutes: 2),
                ChatFixtures.message("m4", minutes: 3),
                Self.system("s0", nil, minutes: 4),
            ], pageSize: 50)

        XCTAssertEqual(timeline.rows.map(\.id), ["m1", "s1", "m3", "m4"])
        let rows = timeline.rows
        XCTAssertEqual(rows[1].kind, .system)
        XCTAssertNotNil(rows[1].moment)
        XCTAssertTrue(rows[1].startsRun && rows[1].endsRun)
        XCTAssertTrue(rows[0].endsRun)
        XCTAssertTrue(rows[2].startsRun)
        XCTAssertFalse(rows[2].endsRun)
        XCTAssertNotNil(rows[0].dayLabel(now: ChatFixtures.epoch, calendar: Self.calendar, locale: Self.locale))
        XCTAssertNil(rows[1].dayLabel(now: ChatFixtures.epoch, calendar: Self.calendar, locale: Self.locale))
    }

    func testAMomentOnANewDayGetsADayLabel() {
        var timeline = ChatTimeline(viewerID: ChatFixtures.viewerID)
        timeline.mergeNewest(
            [ChatFixtures.message("m1"), Self.system("s1", .cabalCreated(.init(kind: .cabalCreated)), minutes: 2_000)],
            pageSize: 50)

        XCTAssertTrue(timeline.rows[1].startsDay)
        XCTAssertNotNil(
            timeline.rows[1].dayLabel(
                now: ChatFixtures.epoch.addingTimeInterval(2_000 * 60), calendar: Self.calendar, locale: Self.locale))
    }

    func testMomentCreatedDecodesAndAnUnknownKindIsDropped() async throws {
        let message = Self.system("s1", Self.fill())
        let data = Data(try ChatFixtures.json(message).utf8)
        XCTAssertEqual(ChatEventDecoder.decode(name: "moment.created", data: data), .momentCreated(message))

        let unknown = String(decoding: data, as: UTF8.self).replacingOccurrences(
            of: "trade_filled", with: "something_new")
        XCTAssertNil(ChatEventDecoder.decode(name: "moment.created", data: Data(unknown.utf8)))
    }

    func testReadsAskForMomentsAndALiveMomentDoesNotRaiseArrival() async throws {
        let arrivals = Mutex(0)
        let transport = StubTransport(scripted: [try ChatFixtures.page([ChatFixtures.message("m1")])])
        let session = ChatSession(
            cabalID: ChatFixtures.cabalID,
            viewerID: ChatFixtures.viewerID,
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"),
                transport: PinsAnsweringTransport(inner: transport)),
            realtime: FakeChatRealtime(),
            now: { ChatFixtures.epoch },
            onArrival: { arrivals.withLock { $0 += 1 } }
        )
        await session.open()
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertTrue(paths.contains { $0.contains("moments=true") }, "\(paths)")

        await session.apply(.momentCreated(Self.system("s1", Self.fill(), minutes: 1)))
        await session.apply(.momentCreated(Self.system("s1", Self.fill(), minutes: 1)))
        let state = await ChatFixtures.state(session)

        XCTAssertEqual(ChatFixtures.ids(state), ["m1", "s1"])
        XCTAssertEqual(arrivals.withLock { $0 }, 0)
    }

    func testPricesAreReadOncePerDistinctSymbolAndChunkedAtFifty() async throws {
        let symbols = (1...51).map { "S\($0)x" }
        let rows = Self.rows(symbols + ["S1x"])
        let first = Array(symbols.prefix(50))
        let transport = StubTransport(routes: [
            Self.path(first): [try Self.quotes(first, price: 220_000_000)],
            Self.path(["S51x"]): [try Self.quotes(["S51x"], price: 5_000_000)],
        ])
        let prices = ChatMomentPrices(api: Self.api(transport))

        await prices.refresh(rows: rows)

        XCTAssertEqual(prices.price(for: "S1x"), 220_000_000)
        XCTAssertEqual(prices.price(for: "S51x"), 5_000_000)
        XCTAssertNil(prices.price(for: "other"))
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
    }

    func testAFailedQuoteLeavesThePriceMissing() async throws {
        let transport = StubTransport(routes: [
            Self.path(["NVDAx"]): [ChatFixtures.problem(500, "internal", "boom")]
        ])
        let prices = ChatMomentPrices(api: Self.api(transport))

        await prices.refresh(rows: Self.rows(["NVDAx"]))

        XCTAssertNil(prices.price(for: "NVDAx"))
    }

    func testNoTradeRowsAskForNothing() async {
        let transport = StubTransport(routes: [:])
        let prices = ChatMomentPrices(api: Self.api(transport))

        await prices.refresh(rows: Self.rows([]))

        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    private static func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private static func rows(_ symbols: [String]) -> [ChatRow] {
        var timeline = ChatTimeline(viewerID: ChatFixtures.viewerID)
        let messages = symbols.enumerated().map { index, symbol in
            system("s\(index)", fill(symbol: symbol), minutes: Double(index))
        }
        timeline.mergeNewest(messages + [ChatFixtures.message("m1", minutes: 100)], pageSize: 500)
        return timeline.rows
    }

    private static func path(_ symbols: [String]) -> String {
        "/v1/assets/quotes?symbols=" + symbols.joined(separator: "%2C")
    }

    private static func quotes(_ symbols: [String], price: Int64) throws -> StubTransport.Reply {
        let assets = symbols.map { symbol -> Components.Schemas.AssetSummary in
            var summary = Components.Schemas.AssetSummary.tsla
            summary.symbol = symbol
            summary.priceMicros = price
            return summary
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = try encoder.encode(Components.Schemas.AssetQuotes(assets: assets))
        return .json(.ok, String(decoding: body, as: UTF8.self))
    }
}

@MainActor
extension ChatMomentTests {
    private static let mia = Components.Schemas.ChatAuthor(
        id: ChatFixtures.otherID, handle: nil, displayName: "Mia Chen", photoUrl: nil)

    private static func voteFill(
        _ side: Components.Schemas.TradeFilledMoment.SidePayload = .buy, yes: Int? = 5, no: Int? = 1,
        proposer: Components.Schemas.ChatAuthor? = mia
    ) -> Components.Schemas.ChatMoment {
        .tradeFilled(
            .init(
                kind: .tradeFilled, side: side, symbol: "NVDAx", assetId: "asset-1", priceMicros: 219_400_000,
                usdcMicros: 400_000_000, filledAt: filledAt, swapId: "swap-1", proposalId: "proposal-1",
                yes: yes, no: no, proposer: proposer))
    }

    private static func settled(
        _ outcome: Components.Schemas.ProposalSettledMoment.OutcomePayload = .failed,
        side: Components.Schemas.ProposalSettledMoment.SidePayload = .buy, yes: Int = 2, no: Int = 4,
        usdcMicros: Int64 = 400_000_000, tokenAmount: Int64? = nil
    ) -> Components.Schemas.ChatMoment {
        .proposalSettled(
            .init(
                kind: .proposalSettled, outcome: outcome, proposalId: "proposal-2", side: side, symbol: "NVDAx",
                usdcMicros: usdcMicros, tokenAmount: tokenAmount, yes: yes, no: no, voters: 6, proposer: mia))
    }

    func testAPassedVoteFillReadsTheTallyTheSharesAndWhoCalledIt() async throws {
        let result = try lines(Self.voteFill(), price: 230_000_000)
        XCTAssertEqual(result.title, "Passed 5 to 1 · bought 1.82 NVDA")
        XCTAssertEqual(result.detail, "at $219.40 · Mia called it")
        XCTAssertNil(result.change)
        XCTAssertNil(result.direction)
    }

    func testTheViewerWhoProposedItReadsYouCalledIt() async throws {
        let result = try lines(Self.voteFill(proposer: Self.mia), viewer: ChatFixtures.otherID)
        XCTAssertEqual(result.detail, "at $219.40 · You called it")
    }

    func testAPassedSellReadsSold() async throws {
        let result = try lines(Self.voteFill(.sell, yes: 4, no: 2))
        XCTAssertEqual(result.title, "Passed 4 to 2 · sold 1.82 NVDA")
    }

    func testAVoteFillWithoutAProposerDropsTheCalledIt() async throws {
        let result = try lines(Self.voteFill(proposer: nil))
        XCTAssertEqual(result.detail, "at $219.40")
    }

    func testAFillWithoutATallyReadsAsBefore() async throws {
        let result = try lines(Self.voteFill(yes: nil, no: nil), price: 220_000_000)
        XCTAssertEqual(result.title, "Bought NVDA at $219.40")
        XCTAssertEqual(result.detail, "Filled Sep 30 · now $220.00")
        let moment = try XCTUnwrap(
            ChatMoment(message: Self.system("s", Self.voteFill(yes: nil, no: nil)), viewerID: ""))
        XCTAssertNil(moment.voteSpokenLabel)
    }

    func testAFailedVoteReadsNothingBoughtAndTheProposalHeadline() async throws {
        let result = try lines(Self.settled())
        XCTAssertEqual(result.title, "Failed 2 to 4 · nothing bought")
        XCTAssertEqual(result.detail, "Buy NVDA · $400")
    }

    func testAFailedSellReadsNothingSold() async throws {
        let result = try lines(Self.settled(side: .sell, tokenAmount: 1_500_000))
        XCTAssertEqual(result.title, "Failed 2 to 4 · nothing sold")
        XCTAssertEqual(result.detail, "Sell NVDA · $400")
    }

    func testAnExpiredVoteReadsNotEnoughVotes() async throws {
        let result = try lines(Self.settled(.expired, yes: 1, no: 0))
        XCTAssertEqual(result.title, "Closed 1 to 0 · not enough votes")
        XCTAssertEqual(result.detail, "Buy NVDA · $400")
    }

    func testAVoteWithNoRecordedAmountShowsOnlyTheName() async throws {
        XCTAssertEqual(try lines(Self.settled(usdcMicros: 0)).detail, "Buy NVDA")
    }

    func testTheRowOpensTheProposal() async throws {
        let fill = try XCTUnwrap(ChatMoment(message: Self.system("s", Self.voteFill()), viewerID: ""))
        XCTAssertEqual(fill.proposalID, "proposal-1")
        let settled = try XCTUnwrap(ChatMoment(message: Self.system("s", Self.settled()), viewerID: ""))
        XCTAssertEqual(settled.proposalID, "proposal-2")
        XCTAssertNil(settled.symbol)
        XCTAssertNil(ChatMoment.cabalCreated.proposalID)
        XCTAssertNil(ChatMoment(message: Self.system("s", Self.fill()), viewerID: "")?.proposalID)
    }

    func testTheViewerIsMarkedAsTheProposer() async throws {
        let moment = ChatMoment(message: Self.system("s", Self.settled()), viewerID: ChatFixtures.otherID)
        guard case .proposalSettled(let settled) = moment else { return XCTFail("not settled") }
        XCTAssertTrue(settled.proposerIsMe)
    }

    func testEachVoteMomentHasASpokenLabel() async throws {
        func spoken(_ moment: Components.Schemas.ChatMoment) throws -> String? {
            try XCTUnwrap(ChatMoment(message: Self.system("s", moment), viewerID: "")).voteSpokenLabel
        }
        XCTAssertEqual(
            try spoken(Self.voteFill()),
            "Vote passed, 5 to 1. Bought 1.82 NVDA at 219 dollars 40. Mia called it. Opens the vote.")
        XCTAssertEqual(
            try spoken(Self.settled()),
            "Vote failed, 2 to 4. Nothing bought. Buy NVDA, $400. Opens the vote.")
        XCTAssertEqual(
            try spoken(Self.settled(.expired, side: .sell, yes: 1, no: 0)),
            "Vote closed, 1 to 0. Not enough votes. Nothing sold. Sell NVDA, $400. Opens the vote.")
        XCTAssertNil(ChatMoment.cabalCreated.voteSpokenLabel)
    }

    func testASettledMomentDecodesAndRidesTheTimeline() async throws {
        let message = Self.system("s1", Self.settled())
        let data = Data(try ChatFixtures.json(message).utf8)
        XCTAssertEqual(ChatEventDecoder.decode(name: "moment.created", data: data), .momentCreated(message))
        var timeline = ChatTimeline(viewerID: ChatFixtures.viewerID)
        timeline.mergeNewest([message], pageSize: 50)
        XCTAssertEqual(timeline.rows.map(\.id), ["s1"])
    }
}
