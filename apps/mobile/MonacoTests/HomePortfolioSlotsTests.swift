import Foundation
import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct HomePortfolioSlotsTests {
    @Test func aPortfolioRowOpensItsCabalWithTheSliceUnderTheName() {
        let summary = PortfolioSummary(.sample)
        let row = summary.rows[0]

        #expect(row.share == "60% of your cabals")
        #expect(row.valueMicros == 600_000_000)
        #expect(row.returnBps == 169)
    }

    @Test func everyRangeChipHasASpokenName() {
        let names = LeaderboardRange.allCases.map(\.accessibilityName)

        #expect(names == ["Past hour", "Past day", "Past week", "Past month", "All time"])
    }

    @Test func theFigureRangesAreTheSevenChips() {
        let labels = LeaderboardRange.figurePeriods.map(\.label)
        let names = LeaderboardRange.figurePeriods.map(\.accessibilityName)

        #expect(labels == ["1H", "1D", "1W", "1M", "3M", "6M", "All"])
        #expect(names.suffix(3) == ["Past 3 months", "Past 6 months", "All time"])
    }

    @Test func theCashRowDepositEmphasisDefaultsToProminent() {
        #expect(EnvironmentValues().homeCashEmphasis == .prominent)
    }
}
