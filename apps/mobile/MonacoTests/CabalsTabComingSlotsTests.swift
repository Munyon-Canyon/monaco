import Testing

@testable import Monaco
@testable import MonacoCore

struct CabalsTabComingSlotsTests {
    @Test func bothSlotsAreLive() {
        #expect(CabalsValueChartSlot.isLive)
        #expect(CabalsBoardSlot.isLive)
    }

    @Test func returnSlotIsHiddenOnlyForAViewerWithNoCabal() {
        #expect(!CabalsValueChartSlot.shows(.hidden))
        #expect(CabalsValueChartSlot.shows(.loading))
        #expect(CabalsValueChartSlot.shows(.failed))
        #expect(CabalsValueChartSlot.shows(.loaded))
    }
}
