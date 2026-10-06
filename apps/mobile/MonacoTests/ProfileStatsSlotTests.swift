import Testing

@testable import Monaco

@MainActor
struct ProfileStatsSlotTests {
    @Test func theStatsAndCabalsSlotsAreLive() {
        #expect(ProfileStatsSlot.isLive)
        #expect(ProfileCabalsSlot.isLive)
    }
}
