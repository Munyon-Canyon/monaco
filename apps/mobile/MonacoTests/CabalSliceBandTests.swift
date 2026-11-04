import Testing

@testable import Monaco

@MainActor
struct CabalSliceBandTests {
    @Test
    func theShareFactReadsDollarsAndPercentOfTheTreasury() {
        #expect(
            CabalSliceBand.spoken(share: "$7,000", slice: "35%") == "Your share, $7,000, 35 percent of the treasury")
    }

    @Test
    func aMemberWithNoShareReadsZero() {
        #expect(CabalSliceBand.spoken(share: "$0", slice: "0%") == "Your share, $0, 0 percent of the treasury")
    }

    @Test
    func aTinySliceReadsLessThanOnePercent() {
        #expect(
            CabalSliceBand.spoken(share: "$5", slice: "<1%") == "Your share, $5, less than 1 percent of the treasury")
    }

    @Test
    func aFailedPotReadShowsADashAndSpeaksNoPercent() {
        #expect(CabalSliceBand.unknown == "\u{2013}")
        #expect(CabalSliceBand.spoken(share: CabalSliceBand.unknown, slice: nil) == "Your share, \u{2013}")
    }
}
