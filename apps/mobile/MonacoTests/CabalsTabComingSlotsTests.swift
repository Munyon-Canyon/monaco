import Foundation
import MonacoAPI
import Testing

@testable import Monaco

struct CabalsTabComingSlotsTests {
    @Test func bothSlotsAreLive() {
        #expect(CabalsValueChartSlot.isLive)
        #expect(CabalsBoardSlot.isLive)
    }

    @Test func returnSlotHidesUntilACabalLoads() {
        #expect(!CabalsValueChartSlot.shows(.idle))
        #expect(!CabalsValueChartSlot.shows(.loading))
        #expect(!CabalsValueChartSlot.shows(.failed(.signedOut)))
        #expect(!CabalsValueChartSlot.shows(.loaded([])))
    }

    @Test func returnSlotShowsWithOneCabal() {
        let cabal = Components.Schemas.MyCabal(
            id: "c-1", name: "QA pot", pictureUrl: nil, role: "creator", canVote: true, memberCount: 1,
            joinedAt: Date(), pendingRequestCount: 0)
        #expect(CabalsValueChartSlot.shows(.loaded([cabal])))
    }
}
