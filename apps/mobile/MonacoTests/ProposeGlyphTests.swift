import Testing

@testable import Monaco

@MainActor
struct ProposeGlyphTests {
    @Test func buyAndSellShareTheActivityListsGlyphs() {
        #expect(ProposeGlyph.buy == "arrow.down")
        #expect(ProposeGlyph.sell == "arrow.up")
    }
}
