import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatMomentRowTests {
    private static let filledAt = Date(timeIntervalSince1970: 1_790_778_600)
    private static let now = Date(timeIntervalSince1970: 1_791_028_800)
    private static let viewerID = "viewer"
    private static let nilID = "00000000-0000-0000-0000-000000000000"
    private static let locale = Locale(identifier: "en_US_POSIX")

    private static var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        return calendar
    }

    private static func moment(_ payload: Components.Schemas.ChatMoment) throws -> ChatMoment {
        let message = ChatMessage(
            id: "m1", kind: .system,
            author: .init(id: nilID, handle: nil, displayName: "", photoUrl: nil),
            moment: payload, body: nil, createdAt: filledAt,
            alsoInChannel: false, replyCount: 0, lastRepliers: [], deleted: false, reactions: [], reactionCount: 0)
        return try #require(ChatMoment(message: message, viewerID: viewerID))
    }

    private static func fill(proposalID: String? = nil) -> Components.Schemas.ChatMoment {
        .tradeFilled(
            .init(
                kind: .tradeFilled, side: .buy, symbol: "NVDAx", assetId: "asset-1", priceMicros: 196_750_000,
                usdcMicros: 300_000_000, filledAt: filledAt, swapId: "swap-1", proposalId: proposalID))
    }

    private static func lines(_ moment: ChatMoment, price: Int64?) -> ChatMoment.Lines {
        moment.lines(
            cabalName: "Big Ballers", currentPriceMicros: price, now: now, calendar: calendar, locale: locale)
    }

    private static func spoken(_ moment: ChatMoment, price: Int64?) -> String {
        ChatMomentRow.spokenLabel(
            for: moment, lines: lines(moment, price: price), now: now, calendar: calendar, locale: locale)
    }

    private static func settled() -> Components.Schemas.ChatMoment {
        .proposalSettled(
            .init(
                kind: .proposalSettled, outcome: .failed, proposalId: "proposal-9", side: .buy, symbol: "NVDAx",
                usdcMicros: 400_000_000, tokenAmount: nil, yes: 2, no: 4, voters: 6,
                proposer: .init(id: "u-mia", handle: nil, displayName: "Mia", photoUrl: nil)))
    }

    private static let mia = Components.Schemas.ChatAuthor(
        id: "u-mia", handle: nil, displayName: "Mia", photoUrl: nil)

    private static func passed() -> Components.Schemas.ChatMoment {
        .tradeFilled(
            .init(
                kind: .tradeFilled, side: .buy, symbol: "NVDAx", assetId: "asset-1", priceMicros: 219_400_000,
                usdcMicros: 400_000_000, filledAt: filledAt, swapId: "swap-1", proposalId: "proposal-7", yes: 5,
                no: 1, proposer: mia))
    }

    @Test func aPassedVotesFillIsAPillWithTheTallyAndNoChange() throws {
        let moment = try Self.moment(Self.passed())
        let lines = Self.lines(moment, price: 250_000_000)
        #expect(!ChatMomentRow.isEventLine(moment))
        #expect(lines.title == "Passed 5 to 1 \u{00B7} bought 1.82 NVDA")
        #expect(lines.detail == "at $219.40 \u{00B7} Mia called it")
        #expect(lines.change == nil)
        #expect(lines.direction == nil)
        #expect(ChatMomentRow.destination(of: moment) == .proposal("proposal-7"))
        #expect(
            Self.spoken(moment, price: 250_000_000)
                == "Vote passed, 5 to 1. Bought 1.82 NVDA at 219 dollars 40. Mia called it. Opens the vote.")
    }

    @Test func aFailedVotesPillSaysNothingWasBoughtOverTheProposal() throws {
        let moment = try Self.moment(Self.settled())
        let lines = Self.lines(moment, price: nil)
        #expect(lines.title == "Failed 2 to 4 \u{00B7} nothing bought")
        #expect(lines.detail == "Buy NVDA \u{00B7} $400")
        #expect(lines.change == nil)
    }

    @Test func aFailedSellSaysNothingSold() throws {
        let moment = try Self.moment(
            .proposalSettled(
                .init(
                    kind: .proposalSettled, outcome: .failed, proposalId: "proposal-8", side: .sell, symbol: "AAPLx",
                    usdcMicros: 1_200_000_000, tokenAmount: nil, yes: 1, no: 5, voters: 6, proposer: Self.mia)))
        let lines = Self.lines(moment, price: nil)
        #expect(lines.title == "Failed 1 to 5 \u{00B7} nothing sold")
        #expect(lines.detail == "Sell AAPL \u{00B7} $1,200")
        #expect(ChatMomentRow.destination(of: moment) == .proposal("proposal-8"))
    }

    @Test func aClosedVoteIsAPillThatOpensItsProposal() throws {
        let moment = try Self.moment(Self.settled())
        #expect(!ChatMomentRow.isEventLine(moment))
        #expect(ChatMomentRow.destination(of: moment) == .proposal("proposal-9"))
        #expect(
            Self.spoken(moment, price: nil)
                == "Vote failed, 2 to 4. Nothing bought. Buy NVDA, $400. Opens the vote.")
    }

    @Test func aFillReadsTheWholeSentenceAndSaysItOpensTheTrade() throws {
        let moment = try Self.moment(Self.fill())
        #expect(
            Self.spoken(moment, price: 220_000_000)
                == "Bought NVDA at $196.75. Filled September 30, now $220.00, up 11.8 percent. Opens the trade.")
    }

    @Test func aFillBelowItsPriceReadsDown() throws {
        let moment = try Self.moment(Self.fill())
        #expect(
            Self.spoken(moment, price: 180_000_000)
                == "Bought NVDA at $196.75. Filled September 30, now $180.00, down 8.5 percent. Opens the trade.")
    }

    @Test func aFillWithoutAQuoteReadsTheDateOnly() throws {
        let moment = try Self.moment(Self.fill())
        #expect(
            Self.spoken(moment, price: nil)
                == "Bought NVDA at $196.75. Filled September 30. Opens the trade.")
    }

    @Test func anEventLineReadsItsText() throws {
        let created = try Self.moment(.cabalCreated(.init(kind: .cabalCreated)))
        #expect(Self.spoken(created, price: nil) == "Big Ballers was created")
        let joined = try Self.moment(
            .memberJoined(
                .init(
                    kind: .memberJoined, member: .init(id: Self.viewerID, handle: nil, displayName: "Me", photoUrl: nil)
                )
            ))
        #expect(Self.spoken(joined, price: nil) == "You joined Big Ballers")
    }

    @Test func onlyCreatedAndJoinedAreEventLines() throws {
        #expect(ChatMomentRow.isEventLine(try Self.moment(.cabalCreated(.init(kind: .cabalCreated)))))
        #expect(!ChatMomentRow.isEventLine(try Self.moment(Self.fill())))
    }

    @Test func aFillOpensItsHoldingAndAProposalFillOpensTheProposal() throws {
        #expect(ChatMomentRow.destination(of: try Self.moment(Self.fill())) == .holding(symbol: "NVDAx"))
        #expect(
            ChatMomentRow.destination(of: try Self.moment(Self.fill(proposalID: "p-9"))) == .proposal("p-9"))
    }

    @Test func eventLinesHaveNoTapTarget() throws {
        #expect(ChatMomentRow.destination(of: try Self.moment(.cabalCreated(.init(kind: .cabalCreated)))) == nil)
    }

    @Test func thePercentFollowsTheDirection() {
        #expect(ChatMomentRow.changeColor(.up) == MonacoTheme.gain)
        #expect(ChatMomentRow.changeColor(.down) == MonacoTheme.loss)
        #expect(ChatMomentRow.changeColor(.flat) == MonacoTheme.secondaryText)
        #expect(ChatMomentRow.changeColor(nil) == MonacoTheme.secondaryText)
    }

    @Test func twoEventLinesSitTwelvePointsApart() {
        let own = ChatMomentMetrics.eventLineInset * 2
        #expect(own + ChatMomentRow.topGap(previousIsEventLine: true) == 12)
        #expect(ChatMomentRow.topGap(previousIsEventLine: false) == ChatBubbleMetrics.groupGap)
    }

    @Test func theMetricsMatchTheDesign() {
        #expect(ChatMomentMetrics.logoSize == 28)
        #expect(ChatMomentMetrics.minHeight == 44)
        #expect(ChatMomentMetrics.widthFraction == 0.85)
        #expect(ChatMomentMetrics.badgeSpace == 24)
    }

    @Test func accessibilitySizesFillTheWidth() {
        #expect(ChatMomentRow.fillsWidth(true))
        #expect(!ChatMomentRow.fillsWidth(false))
    }

    @Test func identifiersCarryTheRowID() {
        #expect(ChatMomentRow.identifier(rowID: "m1") == "chat-moment-m1")
    }

    @Test func eventLinesFollowEventLinesOnly() throws {
        let created = try Self.moment(.cabalCreated(.init(kind: .cabalCreated)))
        let fill = try Self.moment(Self.fill())
        #expect(!GroupChatThreadView.followsEventLine(previous: nil, current: created))
        #expect(GroupChatThreadView.followsEventLine(previous: created, current: created))
        #expect(!GroupChatThreadView.followsEventLine(previous: created, current: fill))
        #expect(!GroupChatThreadView.followsEventLine(previous: created, current: nil))
    }
}
