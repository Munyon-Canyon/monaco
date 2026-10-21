import Foundation
import MonacoAPI
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

    @Test func theHeroNamesItselfInEveryPeriod() {
        for range in LeaderboardRange.allCases {
            let figures = HomeHeroFigures(
                amount: "−$417.68", percent: "−3.4%", direction: .down, period: range.returnPeriodPhrase)

            #expect(figures.title == "Investment return · \(range.returnPeriodPhrase)")
            #expect(figures.spokenTitle == "Investment return, \(range.returnPeriodPhrase)")
        }
    }

    @Test func theHeroIsCalledReturnNotBalance() {
        let figures = HomeHeroFigures.resolve(summary: PortfolioSummary(.sample), change: nil, range: .oneMonth)

        #expect(figures.title.hasPrefix("Investment return"))
        #expect(!figures.title.localizedCaseInsensitiveContains("balance"))
        #expect(figures.period == "all time")
    }

    @Test func cashOnlyLeadsWithCashAndOneAction() {
        #expect(HomeCashLead(.amount(500_000_000)) == .ready)
        #expect(HomeCashLead.ready.headline == "Ready to fund a cabal")
        #expect(HomeCashLead.ready.actionTitle == "Choose a cabal")
        #expect(HomeCashLead.ready.explainsPooling)
    }

    @Test func cashOnlyWithNoCashSendsYouToAddMoney() {
        #expect(HomeCashLead(.amount(0)) == .needsMoney)
        #expect(HomeCashLead.needsMoney.actionTitle == "Add money")
        #expect(HomeCashLead.needsMoney.headline == "Add money to fund a cabal")
    }

    @Test func cashThatHasNotLoadedIsNeverShownAsZeroOrFundable() {
        #expect(HomeCashLead(.loading) == .loading)
        #expect(HomeCashLead(.unavailable) == .unavailable)
        for lead in [HomeCashLead.loading, .unavailable] {
            #expect(lead.actionTitle == nil)
            #expect(!lead.explainsPooling)
        }
        #expect(HomeCashLead.unavailable.headline == "We couldn't load your cash.")
    }
}
