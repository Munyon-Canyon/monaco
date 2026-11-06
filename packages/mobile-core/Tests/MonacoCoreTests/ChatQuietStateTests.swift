import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ChatQuietStateTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_791_028_800)

    private func msg(_ id: String, _ kind: ChatMessage.KindPayload, ago: TimeInterval) -> ChatMessage {
        ChatMessage(
            id: id, kind: kind,
            author: .init(id: ChatFixtures.otherID, handle: nil, displayName: "Kai", photoUrl: nil),
            body: "text", createdAt: now.addingTimeInterval(-ago), parentId: nil, alsoInChannel: false,
            replyCount: 0, lastRepliers: [], deleted: false, seenCount: nil, reactions: [], reactionCount: 0)
    }

    private func kind(_ messages: [ChatMessage], open: Int = 0) -> ChatQuietState.Kind {
        ChatQuietState.kind(messages: messages, openProposals: open, now: now)
    }

    func testKinds() async throws {
        XCTAssertEqual(kind([msg("a", .user, ago: 90_000)], open: 1), .none)
        XCTAssertEqual(kind([msg("a", .system, ago: 10)]), .fresh)
        XCTAssertEqual(kind([]), .fresh)
        XCTAssertEqual(kind([msg("a", .user, ago: 86_400 - 60)]), .none)
        XCTAssertEqual(kind([msg("a", .user, ago: 86_400)]), .quiet)
        XCTAssertEqual(kind([msg("a", .proposal, ago: 100_000)]), .quiet)
        XCTAssertEqual(kind([msg("a", .user, ago: 100_000), msg("b", .system, ago: 60)]), .quiet)
        XCTAssertEqual(kind([msg("a", .user, ago: 100_000), msg("b", .user, ago: 60)]), .none)
    }

    func testRowsOverloadAndTitle() async throws {
        XCTAssertEqual(ChatQuietState.kind(rows: [], openProposals: 0, now: now), .fresh)
        XCTAssertEqual(ChatQuietState.title, "No vote open.")
    }

    func testLine() async throws {
        let down = CabalPositionRowFigures(equityUsd: "7.62", dollarPnl: "-0.38", percentReturn: "-0.047")
        XCTAssertEqual(
            ChatQuietState.line(memberCount: 3, pot: "12.38", figures: down),
            "3 members \u{00B7} $12.38 pot \u{00B7} yours $7.62 \u{2212}4.7%")
        XCTAssertEqual(ChatQuietState.line(memberCount: 3, pot: "12.38", figures: nil), "3 members \u{00B7} $12.38 pot")
        XCTAssertEqual(ChatQuietState.line(memberCount: 3, pot: nil, figures: down), "3 members")
        XCTAssertEqual(ChatQuietState.line(memberCount: 1, pot: nil, figures: nil), "1 member")
        let dollars = CabalPositionRowFigures(equityUsd: "7.62", dollarPnl: "-0.38", percentReturn: nil)
        XCTAssertEqual(
            ChatQuietState.line(memberCount: 3, pot: "12.38", figures: dollars),
            "3 members \u{00B7} $12.38 pot \u{00B7} yours $7.62 \u{2212}$0.38")
        let none = CabalPositionRowFigures(equityUsd: "7.62", dollarPnl: "x", percentReturn: nil)
        XCTAssertEqual(
            ChatQuietState.line(memberCount: 2, pot: "12.38", figures: none),
            "2 members \u{00B7} $12.38 pot \u{00B7} yours $7.62")
    }

    func testActions() async throws {
        let voter = ChatQuietState.actions(canPropose: true, voterCount: 3)
        XCTAssertEqual(voter.map(\.title), ["Propose a trade", "Poll", "Share a stock"])
        XCTAssertEqual(
            voter.map(\.line),
            [
                "Pick a stock and an amount, then the three of you vote.",
                "Ask the cabal something.",
                "Drop a ticker into the chat.",
            ])
        XCTAssertEqual(
            ChatQuietState.actions(canPropose: false, voterCount: 3).map(\.title), ["Poll", "Share a stock"])
        let lines = [1, 2, 10, 11].map { ChatQuietState.actions(canPropose: true, voterCount: $0)[0].line }
        XCTAssertEqual(
            lines,
            [
                "Pick a stock and an amount, then you vote.",
                "Pick a stock and an amount, then the two of you vote.",
                "Pick a stock and an amount, then the ten of you vote.",
                "Pick a stock and an amount, then everyone votes.",
            ])
    }

    func testPlaceholder() async throws {
        XCTAssertEqual(
            ChatQuietState.composerPlaceholder(kind: .fresh, cabalName: "Big Ballers"), "Message Big Ballers")
        XCTAssertEqual(
            ChatQuietState.composerPlaceholder(kind: .quiet, cabalName: "Big Ballers"), "Message Big Ballers")
        XCTAssertEqual(
            ChatQuietState.composerPlaceholder(kind: .none, cabalName: "Big Ballers"), "Message")
        XCTAssertEqual(
            ChatQuietState.composerPlaceholder(kind: .quiet, cabalName: "  "), "Message")
        XCTAssertEqual(
            ChatQuietState.composerPlaceholder(kind: .fresh, cabalName: nil), "Message")
    }
}
