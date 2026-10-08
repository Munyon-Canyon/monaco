import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

/// Where the rules and the curve sit in the Stocks tab's rows and the stock screen's sections.
struct StockRowLayoutTests {
    /// 56pt with 4pt either side cut "2 cabals · your slice $294.70" to "your slice $29…".
    @Test func theRowSparklineIs48Wide() {
        #expect(Sparkline.rowWidth == 48)
    }

    /// The second line ends on the member's own slice; it wraps before it truncates.
    @Test func theSecondLineWrapsRatherThanCuttingOffTheSlice() {
        #expect(StockListRow.subtitleLineLimit == 2)
    }

    /// A stock row's rule starts under its text: the row pads by the gutter, then draws
    /// the mark and its gap, and its rule is inset by exactly that.
    @Test func aRuleAfterAMarkStartsWhereTheTextDoes() {
        let rule = MonacoRowLayout(dynamicTypeSize: .large).separatorLeadingInset(markSize: StockListRow.markSize)
        #expect(StockListRow.textLeading == rule)
    }
}

#if DEBUG
/// `-MonacoAssetDetailScroll` / `-MonacoStocksTabScroll`: where a harness opens its page.
struct SampleScrollAnchorTests {
    private let flag = "-MonacoAssetDetailScroll"

    @Test func withoutTheFlagTheScreenOpensAsTheAppDoes() {
        #expect(SampleScrollAnchor.requested(by: flag, in: ["Monaco", "-MonacoAssetDetailSample", "cabals"]) == nil)
        #expect(SampleScrollAnchor.requested(by: flag, in: ["Monaco", flag]) == nil)
    }

    /// Leading anchors, so the sideways rows on the page (range chips, movers) stay put.
    @Test func namedPlacesAreLeadingAnchors() {
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "center"]) == .leading)
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "bottom"]) == .bottomLeading)
    }

    @Test func aFractionIsThatFarDown() {
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "0.25"]) == UnitPoint(x: 0, y: 0.25))
    }

    @Test func anythingElseIsIgnored() {
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "top"]) == nil)
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "1.5"]) == nil)
        #expect(SampleScrollAnchor.requested(by: flag, in: [flag, "-0.2"]) == nil)
    }
}
#endif
