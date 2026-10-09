import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ProfileStatsSlotTests {
    @Test func theStatsAndCabalsSlotsAreLive() {
        #expect(ProfileStatsSlot.isLive)
        #expect(ProfileCabalsSlot.isLive)
    }

    @Test func theBandStacksAtAccessibilitySizesOnly() {
        #expect(ProfileStatsBand.isStacked(.accessibility3))
        #expect(ProfileStatsBand.isStacked(.accessibility1))
        #expect(!ProfileStatsBand.isStacked(.xxxLarge))
        #expect(!ProfileStatsBand.isStacked(.large))
    }
}
