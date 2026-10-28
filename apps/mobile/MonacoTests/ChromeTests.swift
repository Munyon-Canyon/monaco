import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ChromeTests {
    @Test func tabsKeepTheirOrderAndIdentifiers() {
        #expect(MainTab.allCases == [.home, .cabals, .stocks, .feed, .profile])
        #expect(
            MainTab.allCases.map(\.accessibilityIdentifier)
                == ["tab-home", "tab-cabals", "tab-assets", "tab-feed", "tab-profile"])
        #expect(MainTab.allCases.map(\.title) == ["Home", "Cabals", "Markets", "Feed", "Profile"])
    }

    @Test func tabsUseFSymbols() {
        #expect(
            MainTab.allCases.map(\.systemImage)
                == ["house", "person.2", "chart.xyaxis.line", "newspaper", "person.crop.circle"])
    }

    @Test func aBadgeOfZeroShowsNothing() {
        let navigator = AppNavigator()
        #expect(navigator.tabBadges.isEmpty)
        for tab in MainTab.allCases { #expect((navigator.tabBadges[tab] ?? 0) == 0) }
        navigator.tabBadges[.cabals] = 1
        #expect(navigator.tabBadges[.cabals] == 1)
        #expect((navigator.tabBadges[.home] ?? 0) == 0)
    }

    @Test func reviewAndVotesAreHalfAtRegularSizes() {
        for kind in [MonacoSheetDetents.review, .votes] {
            #expect(kind.detents(fittedHeight: 0, dynamicTypeSize: .large) == [.medium])
            #expect(kind.detents(fittedHeight: 300, screenHeight: 852, dynamicTypeSize: .large) == [.medium])
        }
    }

    @Test func everySheetAddsLargeAtAccessibilitySizes() {
        for kind in [MonacoSheetDetents.review, .votes, .result, .fitted] {
            #expect(
                kind.detents(fittedHeight: 0, screenHeight: 852, dynamicTypeSize: .accessibility1)
                    == [.medium, .large])
        }
        #expect(
            MonacoSheetDetents.fitted.detents(fittedHeight: 200, screenHeight: 852, dynamicTypeSize: .accessibility1)
                == [.height(200), .large])
    }

    @Test func fittedUnderHalfReturnsTheFittedHeight() {
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 240, screenHeight: 852) == [.height(240)])
        #expect(MonacoSheetDetents.result.detents(fittedHeight: 240, screenHeight: 852) == [.height(240)])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 425, screenHeight: 852) == [.height(425)])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 430, screenHeight: 852) == [.medium])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 0, screenHeight: 852) == [.medium])
    }

    @Test func theGlassButtonHitAreaIsAtLeast44AtEveryStyleAndSize() {
        let styles: [MonacoGlassButton.Style] = [.glass, .quiet, .selected, .onCover]
        let sizes: [MonacoGlassButton.Size] = [.regular, .compact]
        let placements: [MonacoGlassButton.Placement] = [.content, .toolbar]
        for style in styles {
            for size in sizes {
                for placement in placements {
                    let button = MonacoGlassButton(
                        systemImage: "xmark", accessibilityLabel: "Close", style: style, placement: placement,
                        size: size
                    ) {}
                    let host = UIHostingController(rootView: button)
                    let fitted = host.sizeThatFits(in: CGSize(width: 300, height: 300))
                    #expect(fitted.width >= 44)
                    #expect(fitted.height >= 44)
                }
            }
        }
    }
}
