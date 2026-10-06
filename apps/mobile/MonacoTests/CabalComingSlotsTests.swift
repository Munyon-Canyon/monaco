import Testing

@testable import Monaco

@MainActor
struct CabalComingSlotsTests {
    @Test func theComingSlotsAreLive() {
        let slots: [any CabalSection.Type] = [
            CabalPotSlot.self, CabalValueChartSlot.self, CabalSliceSlot.self, CabalHoldingsSlot.self,
            CabalMemberBoardSlot.self, CabalPauseSlot.self,
        ]
        #expect(slots.allSatisfy { $0.isLive })
    }

    @Test func cabalScreenOrderIsUnchanged() {
        #expect(
            CabalScreen.sections.map { String(describing: $0) } == [
                "CabalHeaderSlot", "CabalPotSlot", "CabalValueChartSlot", "CabalSliceSlot", "CabalPauseSlot",
                "CabalJoinSlot", "CabalActionsSlot", "CabalProposalsSlot", "CabalHoldingsSlot", "CabalAgentSlot",
                "CabalMemberBoardSlot", "CabalActivitySlot",
            ])
    }
}
