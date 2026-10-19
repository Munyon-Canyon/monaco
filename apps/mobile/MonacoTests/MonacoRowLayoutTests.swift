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
            #expect(layout.separatorLeadingInset == 0)
        }
    }

    @Test func aRowIsSeventyTwoPointsTallWithAFortyFourPointMark() {
        #expect(MonacoRowLayout.minHeight == 72)
        #expect(MonacoRowLayout.baseMarkSize == 44)
    }

    @Test func aRowThatKeepsItsRuleUnderTheLabelsDerivesTheInsetFromTheMark() {
        let layout = MonacoRowLayout(dynamicTypeSize: .large)
        #expect(layout.separatorLeadingInset(markSize: 44) == 76)
        let stacked = MonacoRowLayout(dynamicTypeSize: .accessibility3)
        #expect(stacked.separatorLeadingInset(markSize: 44) == MonacoTheme.Space.gutter)
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

    @Test func aTwoLineRowIsSeventyTwoPointsAtTheDefaultSize() {
        #expect(height(subtitle: "60% of your cabals", at: .large) == MonacoRowLayout.minHeight)
    }

    @Test func aOneLineRowIsSeventyTwoPointsAtTheDefaultSize() {
        #expect(height(subtitle: nil, at: .large) == MonacoRowLayout.minHeight)
    }

    @Test func aTwoLineRowGrowsAtAccessibilitySizes() {
        #expect(height(subtitle: "60% of your cabals", at: .accessibility3) > MonacoRowLayout.minHeight)
    }
}

struct MonacoSurfaceComponentTests {
    @Test func noticesPrefixTheirToneForVoiceOver() {
        #expect(MonacoNotice(.warn, message: "Prices are delayed").spokenText == "Warning: Prices are delayed")
        #expect(
            MonacoNotice(.error, title: "Offline", message: "Check your connection.").spokenText
                == "Error: Offline Check your connection.")
        #expect(MonacoNotice(.info, message: "Saved").spokenText == "Saved")
    }

    @Test func theOpensSoonCaptionIsOneSentence() {
        #expect(MonacoOpensSoon().text == "Opens soon")
        #expect(MonacoOpensSoon("Rewards").text == "Rewards opens soon")
    }

    @Test func aPnLPillReadsItsFigureWithTheSharedWording() {
        #expect(PnLPill(text: "+8.2% · 1M", tone: .gain).spokenText == "up 8.2 percent, 1M")
        #expect(PnLPill(text: "P&L private", tone: .muted).spokenText == "P&L private")
    }

    @Test func everyEventKindHasItsOwnTitle() {
        let titles = EventPill.Kind.allCases.map(\.title)
        #expect(Set(titles).count == titles.count)
        #expect(titles.contains("Price move"))
    }

    @Test func cardStylesUseTheCardRadii() {
        #expect(MonacoCard<EmptyView>.Style.standard.radius == MonacoTheme.Radius.card)
        #expect(MonacoCard<EmptyView>.Style.inset.radius == MonacoTheme.Radius.card)
        #expect(MonacoCard<EmptyView>.Style.tight.radius == MonacoTheme.Radius.cardTight)
        #expect(MonacoCard<EmptyView>.Style.inset.verticalPadding == 0)
    }

    @Test func skeletonShapesMatchTheDesign() {
        #expect(SkeletonMetrics.titleHeight == 42)
        #expect(SkeletonMetrics.cardHeight == 126)
        #expect(SkeletonMetrics.lineHeight == 14)
    }
}
