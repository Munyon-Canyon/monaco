import Testing

@testable import Monaco

@MainActor
struct CabalComingSlotsTests {
    @Test func theComingSlotsAreLive() {
        let slots: [any CabalSection.Type] = [
            CabalPotSlot.self, CabalValueChartSlot.self, CabalSliceSlot.self, CabalHoldingsSlot.self,
            CabalPauseSlot.self,
        ]
        #expect(slots.allSatisfy { $0.isLive })
    }

    @Test func cabalScreenOrderPutsTheHeaderFirst() {
        #expect(
            CabalScreen.sections.map { String(describing: $0) } == [
                "CabalHeaderSlot", "CabalClosedSlot", "CabalPauseSlot", "CabalJoinSlot", "CabalPotSlot",
                "CabalSliceSlot",
                "CabalActionsSlot", "CabalProposalsSlot", "CabalTabs",
            ])
    }
}
