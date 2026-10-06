import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct HomePortfolioSlotsTests {
    @Test func aPortfolioRowOpensItsCabalWithTheSliceUnderTheName() {
        let summary = PortfolioSummary(.sample)
        let row = summary.rows[0]

        #expect(row.share == "60.00% of your money")
        #expect(row.valueMicros == 600_000_000)
        #expect(row.returnBps == 169)
    }

    @Test func everyRangeChipHasASpokenName() {
        let names = LeaderboardRange.allCases.map(\.accessibilityName)

        #expect(names == ["Past hour", "Past day", "Past week", "Past month", "All time"])
    }
}
