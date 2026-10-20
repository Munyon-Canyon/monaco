import SwiftUI
import Testing

@testable import Monaco

struct MonacoChartPriceTagTests {
    @Test func tagStartsRightOfTheShortLine() {
        let x = MonacoChartPriceTag.leading(plotMaxX: 120, width: 60, chartWidth: 390)
        #expect(x == 122)
    }

    @Test func tagOnAFullChartStaysInsideTheChart() {
        let x = MonacoChartPriceTag.leading(plotMaxX: 336, width: 54, chartWidth: 390)
        #expect(x == 336)
        #expect(x + 54 <= 390)
    }

    @Test func tagIsClampedOnScreenWhenTheLineReachesTheEdge() {
        let x = MonacoChartPriceTag.leading(plotMaxX: 388, width: 54, chartWidth: 390)
        #expect(x == 336)
    }

    @Test func tagNeverStartsLeftOfTheChart() {
        #expect(MonacoChartPriceTag.leading(plotMaxX: -10, width: 500, chartWidth: 390) == 0)
    }

    @Test func widthCapFollowsTheGutterAndKeepsTheMinimum() {
        #expect(MonacoChartPriceTag.maxWidth(gutter: 56) == 54)
        #expect(MonacoChartPriceTag.maxWidth(gutter: 10) == MonacoChartPriceTag.minWidth)
    }

    @Test func twoPointChartSizesTheTagToItsTextNotTheGutter() {
        let host = UIHostingController(
            rootView: Text("1,923.50")
                .font(MonacoTheme.Typo.stamp)
                .lineLimit(1)
                .padding(.horizontal, MonacoTheme.Space.xs)
                .frame(minWidth: MonacoChartPriceTag.minWidth)
                .frame(height: MonacoChartPriceTag.height))
        let natural = host.sizeThatFits(in: CGSize(width: 390, height: MonacoChartPriceTag.height))
        #expect(natural.width < 390 - 120)
        #expect(natural.width >= MonacoChartPriceTag.minWidth)
        #expect(natural.height == MonacoChartPriceTag.height)
    }
}
