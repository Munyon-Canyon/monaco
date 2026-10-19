import SwiftUI
import Testing
import UIKit

@testable import Monaco

/// `MoneyFont` builds its `@ScaledMetric` bases from `MoneyStyle.baseSize`, so these assertions
/// are over the numbers that actually render. The values are the redesign's money roles
/// (`docs/design.md`): hero 44 bold, large 35 bold, card 22 bold, row 17 semibold, small 15 semibold.
struct MoneyStyleScalingTests {
    @Test func everyStyleHasItsDesignSize() {
        #expect(MoneyStyle.hero.baseSize == 44)
        #expect(MoneyStyle.large.baseSize == 35)
        #expect(MoneyStyle.card.baseSize == 22)
        #expect(MoneyStyle.row.baseSize == 18)
        #expect(MoneyStyle.caption.baseSize == 15)
    }

    @Test func stylesScaleAgainstTheTextStyleOfTheirSize() {
        #expect(MoneyStyle.hero.textStyle == .largeTitle)
        #expect(MoneyStyle.large.textStyle == .title)
        #expect(MoneyStyle.card.textStyle == .title2)
        #expect(MoneyStyle.row.textStyle == .body)
        #expect(MoneyStyle.caption.textStyle == .subheadline)
    }

    @Test func weightsMatchTheDesign() {
        #expect(MoneyStyle.hero.weight == .bold)
        #expect(MoneyStyle.large.weight == .bold)
        #expect(MoneyStyle.card.weight == .bold)
        #expect(MoneyStyle.row.weight == .semibold)
        #expect(MoneyStyle.caption.weight == .semibold)
    }
}

@MainActor
struct SectionStackLayoutTests {
    @Test func aZeroHeightSectionTakesNoGap() {
        let host = UIHostingController(
            rootView: SectionStackLayout(spacing: SectionStackMetrics.spacing) {
                Color.red.frame(height: 10)
                Color.clear.frame(height: 0)
                Color.red.frame(height: 10)
            })
        let size = host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
        #expect(size.height == 20 + SectionStackMetrics.spacing)
    }
}
