import Testing

@testable import Monaco

@MainActor
struct CabalSliceBandTests {
    @Test
    func aMemberWithNoShareStillGetsTheStatRow() {
        let items = CabalSliceBand.placeholderItems(openVotes: 2, share: "$0", slice: "0%")
        #expect(items.prefix(3).map(\.label) == ["Your share", "Of treasury", "Open votes"])
        #expect(items.prefix(3).map(\.value) == ["$0", "0%", "2"])
    }

    @Test
    func aFailedPotReadKeepsTheRowWithDashes() {
        let items = CabalSliceBand.placeholderItems(
            openVotes: 0, share: CabalSliceBand.unknown, slice: CabalSliceBand.unknown)
        #expect(items.prefix(3).map(\.value) == ["\u{2013}", "\u{2013}", "0"])
    }
}
