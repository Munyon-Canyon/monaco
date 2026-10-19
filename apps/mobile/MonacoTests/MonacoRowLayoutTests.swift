import SwiftUI
import Testing
import UIKit

@testable import Monaco

struct MonacoRowLayoutTests {
    @Test func staysInlineAtEveryNonAccessibilitySize() {
        let sizes: [DynamicTypeSize] = [.xSmall, .small, .medium, .large, .xLarge, .xxLarge, .xxxLarge]
        for size in sizes {
            let layout = MonacoRowLayout(dynamicTypeSize: size)
            #expect(layout.isStacked == false)
            #expect(layout.titleLineLimit == 1)
            #expect(layout.separatorLeadingInset == 68)
        }
    }

    @Test func stacksAtAccessibilitySizes() {
        let sizes: [DynamicTypeSize] = [
            .accessibility1, .accessibility2, .accessibility3, .accessibility4, .accessibility5,
        ]
        for size in sizes {
            let layout = MonacoRowLayout(dynamicTypeSize: size)
            #expect(layout.isStacked)
            #expect(layout.titleLineLimit == 2)
            #expect(layout.minimumTitleWidth == nil)
        }
    }

    @Test func inlineRowsKeepAFloorForTheTitle() {
        let layout = MonacoRowLayout(dynamicTypeSize: .large)
        #expect(layout.minimumTitleWidth == MonacoRowLayout.baseMinimumTitleWidth)
        #expect(layout.minimumTitleWidth == 96)
    }

    @Test func theTitleFloorScalesWithTheTitle() {
        let scaled = MonacoRowLayout(dynamicTypeSize: .xxxLarge, scaledTitleWidthFloor: 130)
        #expect(scaled.minimumTitleWidth == 130)
        #expect(scaled.isStacked == false)
    }

    @Test func aStackedRowHasNoTitleFloorEvenWhenOneIsPassedIn() {
        let stacked = MonacoRowLayout(dynamicTypeSize: .accessibility3, scaledTitleWidthFloor: 130)
        #expect(stacked.minimumTitleWidth == nil)
    }
}

@MainActor
struct MonacoRowHeightTests {
    private func height(subtitle: String?, at size: DynamicTypeSize) -> CGFloat {
        let host = UIHostingController(
            rootView: MonacoRow(title: "Alpha", subtitle: subtitle) {
                Color.clear
            } trailing: {
                Text("$600.00")
                Text("+1.69%")
            }
            .dynamicTypeSize(size))
        return host.sizeThatFits(in: CGSize(width: 402, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func aTwoLineRowIsSixtyFourPointsAtTheDefaultSize() {
        #expect(height(subtitle: "60% of your cabals", at: .large) == MonacoRowLayout.minHeight)
    }

    @Test func aOneLineRowIsSixtyFourPointsAtTheDefaultSize() {
        #expect(height(subtitle: nil, at: .large) == MonacoRowLayout.minHeight)
    }

    @Test func aTwoLineRowGrowsAtAccessibilitySizes() {
        #expect(height(subtitle: "60% of your cabals", at: .accessibility3) > MonacoRowLayout.minHeight)
    }
}
