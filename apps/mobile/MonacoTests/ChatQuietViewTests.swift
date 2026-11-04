import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatQuietViewTests {
    private static func pot(
        value: Int64 = 12_380_000, mine: Int64 = 7_620_000, net: Int64 = 8_000_000, pnl: Int64 = -375_000
    ) -> Components.Schemas.CabalPot {
        Components.Schemas.CabalPot.sample(
            potValueMicros: value, cashMicros: value, cashWeightBps: 10_000, pnlMicros: pnl, holdings: [],
            me: .init(
                shareUnits: mine, valueMicros: mine, sliceBps: 6_000, netContributedMicros: net, pnlMicros: pnl))
    }

    private static func content(kind: ChatQuietState.Kind) -> ChatQuietContent {
        ChatQuietContent(
            kind: kind, cabalID: "c1", cabalName: "Big Ballers", pictureURL: nil, line: "3 members",
            actions: ChatQuietState.actions(canPropose: true, voterCount: 3), search: nil,
            focusComposer: {}, openPoll: {}, sendStock: { _ in })
    }

    @Test func theLineReadsTheMembersThePotAndTheViewersShare() {
        let facts = ChatQuietFacts(Self.pot())
        #expect(facts.line(memberCount: 3) == "3 members \u{00B7} $12.38 pot \u{00B7} yours $7.62 \u{2212}4.7%")
    }

    @Test func theLineShowsOnlyWhatIsKnownWhileTheFiguresLoad() {
        #expect(ChatQuietFacts().line(memberCount: 3) == "3 members")
        #expect(ChatQuietFacts().line(memberCount: 1) == "1 member")
        let potOnly = ChatQuietFacts(pot: "12.380000")
        #expect(potOnly.line(memberCount: 3) == "3 members \u{00B7} $12.38 pot")
    }

    @Test func aViewerWithNothingPutInGetsNoPercentFromADivisionByZero() {
        let facts = ChatQuietFacts(Self.pot(mine: 0, net: 0, pnl: 0))
        #expect(facts.line(memberCount: 2).hasPrefix("2 members \u{00B7} $12.38 pot \u{00B7} yours $0.00"))
    }

    @Test func microsBecomeADecimalString() {
        #expect(ChatQuietFacts.decimal(micros: 12_380_000) == "12.380000")
        #expect(ChatQuietFacts.decimal(micros: -375_000) == "-0.375000")
        #expect(ChatQuietFacts.decimal(micros: 0) == "0.000000")
    }

    @Test func aLossPercentIsRedAndAGainIsGreen() {
        let loss = ChatQuietHeader.styled("3 members \u{00B7} $12.38 pot \u{00B7} yours $7.62 \u{2212}4.7%")
        let colored = loss.runs.filter { $0.foregroundColor != nil }
        #expect(colored.count == 1)
        #expect(colored.first?.foregroundColor == MonacoTheme.loss)
        #expect(String(loss[colored[colored.startIndex].range].characters) == "\u{2212}4.7%")
        let gain = ChatQuietHeader.styled("3 members \u{00B7} $12.38 pot \u{00B7} yours $7.62 +4.7%")
        #expect(gain.runs.contains { $0.foregroundColor == MonacoTheme.gain })
    }

    @Test func aLineWithNoChangeHasNoColour() {
        let plain = ChatQuietHeader.styled("3 members \u{00B7} $12.38 pot")
        #expect(!plain.runs.contains { $0.foregroundColor != nil })
        #expect(!ChatQuietHeader.styled("3 members").runs.contains { $0.foregroundColor != nil })
    }

    @Test func theSceneIs210PointsAndShrinksAboveTheKeyboard() {
        #expect(ChatQuietMetrics.sceneHeight(container: 700) == 210)
        #expect(ChatQuietMetrics.sceneHeight(container: 400) < 210)
        #expect(ChatQuietMetrics.sceneHeight(container: 100) == ChatQuietMetrics.sceneMinimumHeight)
    }

    @Test func aFreshChatShowsTheSceneAndTheRowsAQuietOneOnlyTheRows() {
        let fresh = Self.content(kind: .fresh)
        #expect(fresh.showsScene && fresh.showsActions)
        let quiet = Self.content(kind: .quiet)
        #expect(!quiet.showsScene && quiet.showsActions)
        let none = Self.content(kind: .none)
        #expect(!none.showsScene && !none.showsActions)
    }

    @Test func theRowsHaveTheirIdentifiersAndReadTitleThenLine() {
        let actions = ChatQuietState.actions(canPropose: true, voterCount: 3)
        let identifiers: [String] = actions.map { ChatQuietRow.identifier($0.id) }
        #expect(identifiers == ["chat-quiet-propose", "chat-quiet-poll", "chat-quiet-stock"])
        #expect(
            ChatQuietRow.spoken(actions[0])
                == "Propose a trade. Pick a stock and an amount, then the three of you vote.")
    }

    @Test func aMemberWhoCannotProposeSeesOnlyPollAndShareAStock() {
        let actions = ChatQuietState.actions(canPropose: false, voterCount: 3)
        #expect(actions.map(\.id) == [.poll, .shareStock])
    }

    @Test func startTheConversationIsGone() {
        #expect(ChatQuietState.composerPlaceholder(kind: .fresh, cabalName: "Big Ballers") == "Message Big Ballers")
        #expect(ChatQuietState.composerPlaceholder(kind: .quiet, cabalName: "Big Ballers") == "Message Big Ballers")
        #expect(ChatQuietState.composerPlaceholder(kind: .none, cabalName: "Big Ballers") == "Message")
    }
}
