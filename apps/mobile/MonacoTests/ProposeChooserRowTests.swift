import Testing

@testable import Monaco

@MainActor
struct ProposeChooserRowTests {
    @Test func buyAndSellShareTheActivityListsGlyphs() {
        #expect(ProposeChooserRow.buy == "arrow.down")
        #expect(ProposeChooserRow.sell == "arrow.up")
    }
}
