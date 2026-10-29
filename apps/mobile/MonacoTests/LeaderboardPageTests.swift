import Testing

@testable import MonacoCore

struct LeaderboardPageTests {
    @Test func movementIsSpelledOutNotDrawnAsArrows() {
        #expect(LeaderboardCopy.spoken(.down(2)) == "Down 2 places from the previous snapshot")
        #expect(LeaderboardCopy.spoken(.unchanged) == "Unchanged")
        #expect(LeaderboardCopy.spoken(.new) == "New on the board")
    }
}
