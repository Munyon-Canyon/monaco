import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

struct LeaderboardPageTests {
    private func entry(
        movement: LeaderboardMovement?, compares: Bool = true, members: Int? = 12, rank: Int32 = 1
    ) -> LeaderboardPageRow {
        LeaderboardPageRow(
            row: LeaderboardRowView(
                .sample(
                    rank: rank, id: "ape", name: "Ape Society", kind: .cabal, pnlMicros: 12_690_000_000,
                    returnBps: 4_230)),
            movement: movement, comparesMovement: compares, memberCount: members)
    }

    @Test func aRowSpeaksRankMovementNameMembersAndFigures() {
        #expect(
            LeaderboardView.spoken(entry(movement: .up(4)), board: .cabals)
                == "Rank 1, Up 4 places from the previous snapshot, Ape Society, 12 members, up 42.30 percent, up 12690 dollars"
        )
    }

    @Test func movementIsSpelledOutNotDrawnAsArrows() {
        #expect(LeaderboardCopy.spoken(.down(2)) == "Down 2 places from the previous snapshot")
        #expect(LeaderboardCopy.spoken(.unchanged) == "Unchanged")
        #expect(LeaderboardCopy.spoken(.new) == "New on the board")
    }

    @Test func anotherSortSaysMovementIsNotCompared() {
        let spoken = LeaderboardView.spoken(entry(movement: .up(4), compares: false), board: .cabals)

        #expect(spoken.contains("Rank movement not compared for this sort"))
        #expect(!spoken.contains("Up 4"))
    }

    @Test func aLiveRowWithoutMovementSpeaksNone() {
        let spoken = LeaderboardView.spoken(entry(movement: nil, members: nil), board: .cabals)

        #expect(spoken.hasPrefix("Rank 1, Ape Society, up 42.30 percent"))
    }
}
