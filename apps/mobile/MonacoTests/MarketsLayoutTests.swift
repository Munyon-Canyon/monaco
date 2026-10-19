import SwiftUI
import Testing

@testable import Monaco

struct MarketsLayoutTests {
    @Test func aMoverTileIs244WideAtTheDefaultTextSize() {
        #expect(MoverTile.width(for: .large) == 244)
        #expect(MoverTile.width(for: .xxLarge) == 244)
    }

    @Test func aMoverTileGrowsTo285FromXXXLarge() {
        #expect(MoverTile.width(for: .xxxLarge) == 285)
    }

    @Test func atAccessibilityTextSizesTheRailStacksAndTilesTakeTheFullWidth() {
        for size in [DynamicTypeSize.accessibility1, .accessibility3, .accessibility5] {
            #expect(MoverTile.width(for: size) == nil)
        }
    }
}
