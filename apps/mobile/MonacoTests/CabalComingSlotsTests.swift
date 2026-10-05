import Foundation
import MonacoAPI
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

    @Test func membersAreOrderedByJoinDate() {
        let base = Components.Schemas.Cabal.sampleWithMembers(role: "member").members
        let now = Date()
        var kai = base[0]
        var jordan = base[1]
        var priya = base[2]
        kai.joinedAt = now.addingTimeInterval(-60)
        jordan.joinedAt = now.addingTimeInterval(-3_600)
        priya.joinedAt = now
        #expect(CabalMemberBoardSlot.ordered([priya, kai, jordan]).map(\.handle) == ["jordan", "kai", "priya"])
    }

    @Test func theCreatorLeadsATie() {
        let members = Components.Schemas.Cabal.sampleWithMembers(role: "member").members
        let creator = members[0]
        #expect(creator.role == "creator")
        #expect(
            CabalMemberBoardSlot.ordered([members[1], members[2], creator]).map(\.handle) == ["kai", "jordan", "priya"])
    }
}
