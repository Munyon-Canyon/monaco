import Foundation
import MonacoAPI
import Testing

@testable import Monaco

@MainActor
struct ProfileStatsSlotTests {
    @Test func theStatsAndCabalsSlotsAreLive() {
        #expect(ProfileStatsSlot.isLive)
        #expect(ProfileCabalsSlot.isLive)
    }

    @Test func theCabalsColumnCountsTheMembersCabals() {
        #expect(ProfileStatsSlot.cabalsCount(.loaded([cabal("c-1"), cabal("c-2")])) == "2")
        #expect(ProfileStatsSlot.cabalsCount(.loaded([])) == "0")
    }

    @Test func theCabalsColumnHasNoValueUntilTheCabalsLoad() {
        #expect(ProfileStatsSlot.cabalsCount(.idle) == nil)
        #expect(ProfileStatsSlot.cabalsCount(.loading) == nil)
        #expect(ProfileStatsSlot.cabalsCount(.failed(.signedOut)) == nil)
    }

    private func cabal(_ id: String) -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: id, name: "Cabal \(id)", pictureUrl: nil, role: "member", canVote: true, memberCount: 3,
            joinedAt: Date(), pendingRequestCount: 0, unreadCount: 0)
    }
}
