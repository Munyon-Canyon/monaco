import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco
@testable import MonacoCore

struct LeaderboardRowEmphasisTests {
    private func entry(returnBps: Int64? = 4_230, pnlMicros: Int64 = 12_690_000_000) -> LeaderboardPageRow {
        LeaderboardPageRow(
            row: LeaderboardRowView(
                .sample(
                    rank: 2, id: "ape", name: "Ape Society", kind: .cabal, pnlMicros: pnlMicros,
                    returnBps: returnBps)),
            movement: nil, comparesMovement: true, memberCount: nil)
    }

    private func figure(for sort: LeaderboardSort) -> BoardRow<EmptyView>.Figure {
        LeaderboardView.figure(for: sort)
    }

    @Test func sortByReturnMakesTheReturnThePrimaryFigure() {
        #expect(figure(for: .returnPercent) == .returnPercent)
    }

    @Test func sortByDollarPnlMakesTheDollarsThePrimaryFigure() {
        #expect(figure(for: .pnlAmount) == .pnl)
    }

    @Test func sortByNameKeepsTheReturnPrimaryBecauseRankComesFromIt() {
        #expect(figure(for: .name) == .returnPercent)
    }

    @Test func sortByReturnSpeaksTheReturnBeforeTheDollars() {
        let spoken = LeaderboardView.spoken(entry(), board: .people, sort: .returnPercent)

        #expect(spoken.hasSuffix("up 42.30 percent, up 12690 dollars"))
    }

    @Test func sortByDollarPnlSpeaksTheDollarsBeforeTheReturn() {
        let spoken = LeaderboardView.spoken(entry(), board: .people, sort: .pnlAmount)

        #expect(spoken.hasSuffix("up 12690 dollars, up 42.30 percent"))
    }

    @Test func aMissingReturnIsStillSaidUnderEitherSort() {
        let row = entry(returnBps: nil)

        #expect(LeaderboardView.spoken(row, board: .people, sort: .returnPercent).contains("no return yet"))
        #expect(LeaderboardView.spoken(row, board: .people, sort: .pnlAmount).hasSuffix("no return yet"))
    }

    @Test func theDefaultSpeechIsTheReturnFirstOrder() {
        let row = entry()

        #expect(
            LeaderboardView.spoken(row, board: .people)
                == LeaderboardView.spoken(row, board: .people, sort: .returnPercent))
    }

    @Test func aReturnFirstRowSpeaksTheReturnThenTheDollars() {
        let spoken = BoardRow<EmptyView>.spoken(entry().row, figure: .returnPercent)

        #expect(spoken == "Rank 2, Ape Society, up 42.30 percent, up 12690 dollars")
    }
}
