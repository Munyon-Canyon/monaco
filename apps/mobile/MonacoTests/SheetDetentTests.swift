import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct SheetDetentTests {
    private let kinds: [MonacoSheetDetents] = [.review, .votes, .result, .fitted]

    @Test func noSheetKindOffersTheLargeDetentAtRegularSizes() {
        for kind in kinds {
            for fitted in [0, 180, 900] as [CGFloat] {
                #expect(!kind.detents(fittedHeight: fitted, screenHeight: 800).contains(.large))
                #expect(kind.detents(fittedHeight: fitted, screenHeight: 800).count == 1)
            }
        }
    }

    @Test func reviewAndVotesOpenAtHalfTheScreen() {
        #expect(MonacoSheetDetents.review.detents(fittedHeight: 0) == [.medium])
        #expect(MonacoSheetDetents.votes.detents(fittedHeight: 0) == [.medium])
    }

    @Test func depositConfirmedFitsItsContent() {
        #expect(MonacoSheetDetents.result.detents(fittedHeight: 280, screenHeight: 800) == [.height(280)])
    }

    @Test func aFittedSheetUsesItsContentHeightOrHalfWhileMeasuring() {
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 240, screenHeight: 800) == [.height(240)])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 0, screenHeight: 800) == [.medium])
    }

    @Test func aFittedSheetNeverGrowsPastHalfTheScreen() {
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 395, screenHeight: 800) == [.height(395)])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 405, screenHeight: 800) == [.medium])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 2_000, screenHeight: 800) == [.medium])
    }

    @Test func aSheetWithNoKindOpensAtHalfTheScreen() {
        #expect(MonacoSheetDetents.half == [.medium])
        #expect(!MonacoSheetDetents.half.contains(.large))
    }
}

@MainActor
struct DepositSheetTests {
    @Test func presentingDepositDoesNotPushAScreen() {
        let navigator = AppNavigator()
        navigator.presentDeposit(prefillMicros: 5_000_000, cabalID: "c1")
        #expect(navigator.deposit?.prefillMicros == 5_000_000)
        #expect(navigator.deposit?.cabalID == "c1")
        #expect(navigator.homePath.isEmpty)
    }

    @Test func choosingAnOptionClosesTheSheetAndPushesOnTheCurrentTab() {
        let navigator = AppNavigator()
        navigator.selectedTab = .profile
        navigator.presentDeposit()
        let route = AnyAppRoute(CardFundingRoute(prefillMicros: nil, cabalID: nil))
        navigator.continueDeposit(with: route)
        #expect(navigator.deposit == nil)
        #expect(navigator.profilePath == [route])
        #expect(navigator.homePath.isEmpty)
    }
}
